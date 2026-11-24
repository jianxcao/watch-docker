package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// ErrorType 标识远程查询错误的类型
type ErrorType string

const (
	ErrorTypeNone        ErrorType = ""
	ErrorTypeNotFound    ErrorType = "not_found"
	ErrorTypeRateLimited ErrorType = "rate_limited"
	ErrorTypeGeneral     ErrorType = "general"
)

type CacheEntry struct {
	IndexDigest  string
	ChildDigest  string
	ConfigDigest string
	Expiry       time.Time
	ErrorType    ErrorType // 如果是负缓存，记录错误类型
}

// rateLimitState 记录某个 registry 的限流状态
type rateLimitState struct {
	Until time.Time
}

// DigestQuery 描述单个镜像及目标平台的远程 digest 查询需求
type DigestQuery struct {
	ImageRef         string
	OS               string
	Arch             string
	Variant          string
	NeedConfigDigest bool
}

// Key 返回包含平台信息的查询唯一键
func (q DigestQuery) Key() string {
	osName, arch, variant := normalizePlatform(q.OS, q.Arch, q.Variant)
	return fmt.Sprintf("%s|%s/%s/%s", q.ImageRef, osName, arch, variant)
}

// DigestResult 批量查询的单个结果
type DigestResult struct {
	IndexDigest  string
	ChildDigest  string
	ConfigDigest string
	Error        error
	ErrType      ErrorType
}

type Client struct {
	cache       map[string]CacheEntry
	mu          sync.RWMutex
	rateLimits  map[string]rateLimitState // canonical registry host -> 限流状态
	rateLimitMu sync.RWMutex

	credsMu     sync.RWMutex
	credentials map[string]authn.AuthConfig // canonical host -> auth config

	transportMu    sync.Mutex
	transportProxy string
	httpTransport  *http.Transport

	sf singleflight.Group
}

func New() *Client {
	c := &Client{
		cache:       make(map[string]CacheEntry),
		rateLimits:  make(map[string]rateLimitState),
		credentials: make(map[string]authn.AuthConfig),
	}

	c.UpdateManifestCredentials()
	return c
}

// getTransport 返回复用的 *http.Transport；当代理配置变化时重建并释放旧连接池
func (c *Client) getTransport() *http.Transport {
	proxyStr := ""
	if cfg := config.Get(); cfg != nil {
		proxyStr = strings.TrimSpace(cfg.Proxy.URL)
	}

	c.transportMu.Lock()
	defer c.transportMu.Unlock()

	if c.httpTransport != nil && c.transportProxy == proxyStr {
		return c.httpTransport
	}

	if c.httpTransport != nil {
		c.httpTransport.CloseIdleConnections()
	}

	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}
	if proxyStr != "" {
		if proxyURL, err := url.Parse(proxyStr); err == nil {
			tr.Proxy = http.ProxyURL(proxyURL)
		} else {
			logger.Logger.Warn("解析代理地址失败，回退系统环境变量代理", zap.String("proxy", proxyStr), zap.Error(err))
			tr.Proxy = http.ProxyFromEnvironment
		}
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}

	c.httpTransport = tr
	c.transportProxy = proxyStr
	return tr
}

func (c *Client) getCache(key string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.Expiry) {
		return e, true
	}
	return CacheEntry{}, false
}

func (c *Client) setCache(key string, entry CacheEntry, ttl time.Duration) {
	now := time.Now()
	entry.Expiry = now.Add(ttl)

	c.mu.Lock()
	defer c.mu.Unlock()
	if prev, ok := c.cache[key]; ok && now.Before(prev.Expiry) && entry.ErrorType == ErrorTypeNone && entry.ConfigDigest == "" && prev.ConfigDigest != "" {
		if prev.IndexDigest == entry.IndexDigest && prev.ChildDigest == entry.ChildDigest {
			entry.ConfigDigest = prev.ConfigDigest
		}
	}
	c.cache[key] = entry
	if len(c.cache) > 512 {
		for k, v := range c.cache {
			if now.After(v.Expiry) {
				delete(c.cache, k)
			}
		}
	}
}

func (c *Client) isRegistryRateLimited(registryKey string) bool {
	c.rateLimitMu.RLock()
	defer c.rateLimitMu.RUnlock()
	if state, ok := c.rateLimits[registryKey]; ok {
		return time.Now().Before(state.Until)
	}
	return false
}

func (c *Client) setRegistryRateLimited(registryKey string, cooldown time.Duration) {
	now := time.Now()
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()
	c.rateLimits[registryKey] = rateLimitState{Until: now.Add(cooldown)}
	for k, v := range c.rateLimits {
		if now.After(v.Until) {
			delete(c.rateLimits, k)
		}
	}
	logger.Logger.Warn("registry 已被限流，暂停请求",
		zap.String("registry", registryKey),
		zap.Duration("cooldown", cooldown))
}

// GetRateLimitedRegistries 返回当前处于限流状态的 registry 列表及剩余冷却时间
func (c *Client) GetRateLimitedRegistries() map[string]time.Duration {
	c.rateLimitMu.RLock()
	defer c.rateLimitMu.RUnlock()
	result := make(map[string]time.Duration)
	now := time.Now()
	for key, state := range c.rateLimits {
		if now.Before(state.Until) {
			result[key] = state.Until.Sub(now)
		}
	}
	return result
}

func canonicalHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if idx := strings.IndexByte(h, '/'); idx >= 0 {
		h = h[:idx]
	}
	switch h {
	case "docker.io", "dockerhub", "index.docker.io", "registry-1.docker.io":
		return "index.docker.io"
	default:
		return h
	}
}

func normalizePlatform(osName, arch, variant string) (string, string, string) {
	o := strings.TrimSpace(osName)
	a := strings.TrimSpace(arch)
	v := strings.TrimSpace(variant)
	if o == "" {
		o = "linux"
	}
	if a == "" {
		a = runtime.GOARCH
	}
	return strings.ToLower(o), strings.ToLower(a), strings.ToLower(v)
}

func normalizeImageRef(ref string) (normalized, host, repo, tag string, err error) {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return "", "", "", "", err
	}
	named = reference.TagNameOnly(named)
	host = canonicalHost(reference.Domain(named))
	repo = reference.Path(named)

	if d, ok := named.(reference.Digested); ok {
		tag = d.Digest().String()
	} else if t, ok := named.(reference.NamedTagged); ok {
		tag = t.Tag()
	} else {
		tag = "latest"
	}

	if d, ok := named.(reference.Digested); ok {
		normalized = fmt.Sprintf("%s/%s@%s", host, repo, d.Digest().String())
	} else {
		normalized = fmt.Sprintf("%s/%s:%s", host, repo, tag)
	}
	return normalized, host, repo, tag, nil
}

// UpdateManifestCredentials 从全局配置全量替换认证凭据，支持任意仓库并清除已删除凭据与历史负面缓存
func (c *Client) UpdateManifestCredentials() {
	cfg := config.Get()
	newCreds := make(map[string]authn.AuthConfig)
	if cfg != nil {
		for _, auth := range cfg.Registry.Auth {
			host := canonicalHost(auth.Host)
			if host == "" {
				continue
			}
			if strings.TrimSpace(auth.Username) == "" && strings.TrimSpace(auth.Token) == "" {
				continue
			}
			newCreds[host] = authn.AuthConfig{
				Username: auth.Username,
				Password: auth.Token,
			}
		}
	}

	c.credsMu.Lock()
	c.credentials = newCreds
	c.credsMu.Unlock()

	c.mu.Lock()
	clear(c.cache)
	c.mu.Unlock()

	c.rateLimitMu.Lock()
	clear(c.rateLimits)
	c.rateLimitMu.Unlock()
}

func (c *Client) getCredentialForHost(host string) (authn.AuthConfig, bool) {
	key := canonicalHost(host)
	c.credsMu.RLock()
	defer c.credsMu.RUnlock()
	cred, ok := c.credentials[key]
	return cred, ok
}

func (c *Client) isHostExplicitHTTP(host string) bool {
	target := canonicalHost(host)
	cfg := config.Get()
	if cfg == nil || target == "" {
		return false
	}
	for _, item := range cfg.Registry.Auth {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.Host)), "http://") && canonicalHost(item.Host) == target {
			return true
		}
	}
	return false
}

func extractHostWithoutPort(hostport string) string {
	h := strings.TrimSpace(hostport)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
}

func (c *Client) allowInsecureFallback(host string) bool {
	if c.isHostExplicitHTTP(host) {
		return true
	}
	h := strings.ToLower(extractHostWithoutPort(host))
	if h == "" {
		return false
	}
	if h == "localhost" || !strings.Contains(h, ".") ||
		strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".lan") ||
		strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".home") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	return false
}

// detectErrorType 基于标准 *transport.Error HTTP 状态码和诊断码分类错误
func detectErrorType(err error) ErrorType {
	if err == nil {
		return ErrorTypeNone
	}
	var terr *transport.Error
	if errors.As(err, &terr) {
		if terr.StatusCode == http.StatusNotFound {
			return ErrorTypeNotFound
		}
		if terr.StatusCode == http.StatusTooManyRequests {
			return ErrorTypeRateLimited
		}
		for _, diag := range terr.Errors {
			switch strings.ToUpper(string(diag.Code)) {
			case "MANIFEST_UNKNOWN", "NAME_UNKNOWN":
				return ErrorTypeNotFound
			case "TOOMANYREQUESTS":
				return ErrorTypeRateLimited
			}
		}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "status code 404") || strings.Contains(msg, "404") && strings.Contains(msg, "manifest_unknown") {
		return ErrorTypeNotFound
	}
	if strings.Contains(msg, "status code 429") || strings.Contains(msg, "toomanyrequests") {
		return ErrorTypeRateLimited
	}
	return ErrorTypeGeneral
}

// GetRemoteDigestsBatch 批量获取多个镜像的远程 digest（兼容旧签名）
func (c *Client) GetRemoteDigestsBatch(ctx context.Context, imageRefs []string, isUserCache bool, cacheOnly bool, concurrency int) map[string]DigestResult {
	queries := make([]DigestQuery, 0, len(imageRefs))
	for _, ref := range imageRefs {
		queries = append(queries, DigestQuery{ImageRef: ref})
	}
	raw := c.GetRemoteDigestsBatchWithPlatforms(ctx, queries, isUserCache, cacheOnly, concurrency)
	out := make(map[string]DigestResult, len(imageRefs))
	for _, q := range queries {
		out[q.ImageRef] = raw[q.Key()]
	}
	return out
}

// GetRemoteDigestsBatchWithPlatforms 批量按指定目标平台查询远程 digest
func (c *Client) GetRemoteDigestsBatchWithPlatforms(ctx context.Context, queries []DigestQuery, isUserCache bool, cacheOnly bool, concurrency int) map[string]DigestResult {
	results := make(map[string]DigestResult, len(queries))
	if len(queries) == 0 {
		return results
	}

	cfg := config.Get()
	if concurrency <= 0 {
		if cfg != nil && cfg.Scan.Concurrency > 0 {
			concurrency = cfg.Scan.Concurrency
		} else {
			concurrency = 3
		}
	}
	if concurrency > 64 {
		concurrency = 64
	}

	ttl := 5 * time.Minute
	if cfg != nil && cfg.Scan.CacheTTL > 0 {
		ttl = cfg.Scan.CacheTTL.Duration()
	}
	const rateLimitCooldown = 5 * time.Minute
	const notFoundCacheTTL = 30 * time.Minute

	mirrors := dockercli.EnabledMirrorEndpoints()

	type pendingItem struct {
		query      DigestQuery
		cacheKey   string
		host       string
		normalized string
	}

	pending := make([]pendingItem, 0, len(queries))
	seenQueryKeys := make(map[string]struct{}, len(queries))

	for _, q := range queries {
		qKey := q.Key()
		if _, dup := seenQueryKeys[qKey]; dup {
			continue
		}
		seenQueryKeys[qKey] = struct{}{}

		normalized, host, _, _, err := normalizeImageRef(q.ImageRef)
		if err != nil {
			results[qKey] = DigestResult{Error: err, ErrType: ErrorTypeGeneral}
			continue
		}

		osName, arch, variant := normalizePlatform(q.OS, q.Arch, q.Variant)
		cacheKey := fmt.Sprintf("%s|%s/%s/%s", normalized, osName, arch, variant)

		if isUserCache {
			if entry, ok := c.getCache(cacheKey); ok {
				if entry.ErrorType != ErrorTypeNone {
					results[qKey] = DigestResult{
						Error:   fmt.Errorf("cached error: %s", entry.ErrorType),
						ErrType: entry.ErrorType,
					}
					continue
				}
				// 如果需要 ConfigDigest 但此前缓存未解析该字段，且允许发网络请求，则继续拉取补充
				if !q.NeedConfigDigest || entry.ConfigDigest != "" || cacheOnly {
					results[qKey] = DigestResult{
						IndexDigest:  entry.IndexDigest,
						ChildDigest:  entry.ChildDigest,
						ConfigDigest: entry.ConfigDigest,
					}
					continue
				}
			}
		}

		if cacheOnly {
			results[qKey] = DigestResult{}
			continue
		}

		canMirror := dockercli.IsDockerHubImage(q.ImageRef) && len(mirrors) > 0
		if !canMirror && c.isRegistryRateLimited(host) {
			results[qKey] = DigestResult{
				Error:   fmt.Errorf("registry %s 请求频率超限，等待冷却", host),
				ErrType: ErrorTypeRateLimited,
			}
			continue
		}

		pending = append(pending, pendingItem{
			query:      q,
			cacheKey:   cacheKey,
			host:       host,
			normalized: normalized,
		})
	}

	if len(pending) == 0 {
		return results
	}

	var (
		resMu sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, concurrency)
	)

	for _, item := range pending {
		wg.Add(1)
		go func(it pendingItem) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				resMu.Lock()
				results[it.query.Key()] = DigestResult{Error: ctx.Err(), ErrType: ErrorTypeGeneral}
				resMu.Unlock()
				return
			}
			defer func() { <-sem }()

			sfKey := fmt.Sprintf("%s|cfg=%v", it.cacheKey, it.query.NeedConfigDigest)
			ch := c.sf.DoChan(sfKey, func() (any, error) {
				flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
				defer cancel()

				dr := c.fetchDigest(flightCtx, it.query, it.normalized, it.host, mirrors)
				if dr.Error != nil {
					switch dr.ErrType {
					case ErrorTypeRateLimited:
						c.setRegistryRateLimited(it.host, rateLimitCooldown)
					case ErrorTypeNotFound:
						c.setCache(it.cacheKey, CacheEntry{ErrorType: ErrorTypeNotFound}, notFoundCacheTTL)
					}
					return dr, nil
				}

				c.setCache(it.cacheKey, CacheEntry{
					IndexDigest:  dr.IndexDigest,
					ChildDigest:  dr.ChildDigest,
					ConfigDigest: dr.ConfigDigest,
					ErrorType:    ErrorTypeNone,
				}, ttl)
				return dr, nil
			})

			var finalRes DigestResult
			select {
			case <-ctx.Done():
				finalRes = DigestResult{Error: ctx.Err(), ErrType: ErrorTypeGeneral}
			case res := <-ch:
				if res.Err != nil {
					finalRes = DigestResult{Error: res.Err, ErrType: detectErrorType(res.Err)}
				} else {
					finalRes = res.Val.(DigestResult)
				}
			}

			resMu.Lock()
			results[it.query.Key()] = finalRes
			resMu.Unlock()
		}(item)
	}

	wg.Wait()
	return results
}

func (c *Client) fetchDigest(ctx context.Context, q DigestQuery, normalizedRef, host string, mirrors []dockercli.MirrorEndpoint) DigestResult {
	targetOS, targetArch, targetVariant := normalizePlatform(q.OS, q.Arch, q.Variant)

	// 1. 若配置了 Docker Hub mirror 且镜像属于 docker.io，先尝试通过 mirror 查询（即使官方处于限流冷却也继续走 mirror）
	if dockercli.IsDockerHubImage(q.ImageRef) && len(mirrors) > 0 {
		if indexDig, childDig, configDig, ok := c.queryDigestViaMirrors(ctx, q.ImageRef, mirrors, targetOS, targetArch, targetVariant, q.NeedConfigDigest); ok {
			return DigestResult{
				IndexDigest:  indexDig,
				ChildDigest:  childDig,
				ConfigDigest: configDig,
			}
		}
	}

	if c.isRegistryRateLimited(host) {
		return DigestResult{
			Error:   fmt.Errorf("registry %s 请求频率超限，等待冷却", host),
			ErrType: ErrorTypeRateLimited,
		}
	}

	// 2. 使用 go-containerregistry 查询官方/私有仓库
	ref, err := name.ParseReference(normalizedRef)
	if err != nil {
		// 对非 HTTPS 私有仓库（如 http:// 或局域网自建）可保留原始引用重试
		ref, err = name.ParseReference(q.ImageRef)
		if err != nil {
			return DigestResult{Error: fmt.Errorf("解析镜像引用失败: %w", err), ErrType: ErrorTypeGeneral}
		}
	}

	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(c.getTransport()),
		remote.WithPlatform(v1.Platform{
			OS:           targetOS,
			Architecture: targetArch,
			Variant:      targetVariant,
		}),
	}
	if cred, ok := c.getCredentialForHost(host); ok {
		opts = append(opts, remote.WithAuth(authn.FromConfig(cred)))
	}

	desc, err := remote.Get(ref, opts...)
	if err != nil {
		// 兼容未配置 TLS 的局域网私有仓库或显式 http:// 配置的仓库（禁止对公网域名静默降级明文传输凭据）
		if strings.Contains(host, ":") && c.allowInsecureFallback(host) && strings.Contains(strings.ToLower(err.Error()), "http: server gave http response to https client") {
			if insecRef, rerr := name.ParseReference(normalizedRef, name.Insecure); rerr == nil {
				desc, err = remote.Get(insecRef, opts...)
			}
		}
		if err != nil {
			errType := detectErrorType(err)
			return DigestResult{Error: err, ErrType: errType}
		}
	}

	return extractDigestFromDescriptor(desc, targetOS, targetArch, targetVariant, q.NeedConfigDigest)
}

func extractDigestFromDescriptor(desc *remote.Descriptor, targetOS, targetArch, targetVariant string, needConfig bool) DigestResult {
	indexDigest := desc.Digest.String()
	if desc.MediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			return DigestResult{Error: fmt.Errorf("解析 manifest index 失败: %w", err), ErrType: ErrorTypeGeneral}
		}
		im, err := idx.IndexManifest()
		if err != nil {
			return DigestResult{Error: fmt.Errorf("读取 index manifest 失败: %w", err), ErrType: ErrorTypeGeneral}
		}
		childDesc := selectDescriptorFromIndex(im, targetOS, targetArch, targetVariant)
		var childDigest, configDigest string
		if childDesc != nil {
			childDigest = childDesc.Digest.String()
			if needConfig {
				img, ierr := idx.Image(childDesc.Digest)
				if ierr != nil {
					return DigestResult{Error: fmt.Errorf("获取平台镜像失败: %w", ierr), ErrType: detectErrorType(ierr)}
				}
				cfgHash, cerr := img.ConfigName()
				if cerr != nil {
					return DigestResult{Error: fmt.Errorf("获取平台镜像 config digest 失败: %w", cerr), ErrType: detectErrorType(cerr)}
				}
				configDigest = cfgHash.String()
			}
		} else {
			if needConfig {
				return DigestResult{
					Error:   fmt.Errorf("未找到匹配平台 %s/%s 的子 manifest", targetOS, targetArch),
					ErrType: ErrorTypeGeneral,
				}
			}
			if childDig, derr := parseManifest(desc.Manifest, targetOS, targetArch, targetVariant); derr == nil {
				childDigest = childDig
			}
		}
		return DigestResult{
			IndexDigest:  indexDigest,
			ChildDigest:  childDigest,
			ConfigDigest: configDigest,
		}
	}

	var configDigest string
	if needConfig {
		img, err := desc.Image()
		if err != nil {
			return DigestResult{Error: fmt.Errorf("解析远程镜像失败: %w", err), ErrType: detectErrorType(err)}
		}
		cfgHash, cerr := img.ConfigName()
		if cerr != nil {
			return DigestResult{Error: fmt.Errorf("获取镜像 config digest 失败: %w", cerr), ErrType: detectErrorType(cerr)}
		}
		configDigest = cfgHash.String()
	}
	return DigestResult{
		IndexDigest:  indexDigest,
		ChildDigest:  indexDigest,
		ConfigDigest: configDigest,
	}
}

func selectDescriptorFromIndex(im *v1.IndexManifest, targetOS, targetArch, targetVariant string) *v1.Descriptor {
	if im == nil {
		return nil
	}
	var archMatch *v1.Descriptor
	for i := range im.Manifests {
		m := &im.Manifests[i]
		if m.Platform == nil {
			continue
		}
		pos := strings.ToLower(m.Platform.OS)
		parch := strings.ToLower(m.Platform.Architecture)
		pvar := strings.ToLower(m.Platform.Variant)
		if pos == "" || pos == "unknown" || parch == "" || parch == "unknown" {
			continue
		}
		if pos != targetOS || parch != targetArch {
			continue
		}
		if pvar == targetVariant {
			return m
		}
		if archMatch == nil {
			archMatch = m
		}
	}
	return archMatch
}

func parseManifest(raw []byte, targetOS, targetArch, targetVariant string) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("empty manifest")
	}
	var im v1.IndexManifest
	if err := json.Unmarshal(raw, &im); err != nil {
		return "", fmt.Errorf("decode index: %w", err)
	}
	if desc := selectDescriptorFromIndex(&im, targetOS, targetArch, targetVariant); desc != nil {
		return desc.Digest.String(), nil
	}
	return "", fmt.Errorf("no matching platform manifest in index for %s/%s", targetOS, targetArch)
}
