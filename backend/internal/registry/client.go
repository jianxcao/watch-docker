package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"

	"github.com/distribution/reference"
	manifestpkg "github.com/docker-make/docker-mainifest/pkg/registry"
)

// ErrorType 标识远程查询错误的类型
type ErrorType string

const (
	ErrorTypeNone         ErrorType = ""
	ErrorTypeNotFound     ErrorType = "not_found"
	ErrorTypeRateLimited  ErrorType = "rate_limited"
	ErrorTypeGeneral      ErrorType = "general"
)

type CacheEntry struct {
	Digest    string
	Expiry    time.Time
	ErrorType ErrorType // 如果是负缓存，记录错误类型
}

// rateLimitState 记录某个 registry 的限流状态
type rateLimitState struct {
	Until time.Time
}

type Client struct {
	cache          map[string]CacheEntry
	mu             sync.RWMutex
	manifestClient *manifestpkg.Client
	rateLimits     map[string]rateLimitState // registry key -> 限流状态
	rateLimitMu    sync.RWMutex
}

func New() *Client {
	cfg := config.Get()

	// 创建 manifest 客户端
	var manifestClient *manifestpkg.Client
	var err error
	if cfg != nil && cfg.Proxy.URL != "" {
		manifestClient, err = manifestpkg.NewClientWithProxy(cfg.Proxy.URL)
		if err != nil {
			logger.Logger.Warn("创建 manifest 客户端失败，使用默认配置", zap.Error(err))
			manifestClient = manifestpkg.NewClient()
		}
	} else {
		manifestClient = manifestpkg.NewClient()
	}

	// 设置 logger
	manifestClient.WithLogger(logger.Logger)

	c := &Client{
		cache:          make(map[string]CacheEntry),
		manifestClient: manifestClient,
		rateLimits:     make(map[string]rateLimitState),
	}

	// 初始化凭据
	c.UpdateManifestCredentials()

	return c
}

func (c *Client) getCache(key string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.Expiry) {
		return e, true
	}
	return CacheEntry{}, false
}

func (c *Client) setCache(key, digest string, ttl time.Duration, errType ErrorType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = CacheEntry{Digest: digest, Expiry: time.Now().Add(ttl), ErrorType: errType}
}

// detectErrorType 从 manifest 库返回的错误字符串中识别错误类型
func detectErrorType(err error) ErrorType {
	if err == nil {
		return ErrorTypeNone
	}
	msg := err.Error()
	if strings.Contains(msg, "状态码: 404") || strings.Contains(msg, "MANIFEST_UNKNOWN") {
		return ErrorTypeNotFound
	}
	if strings.Contains(msg, "状态码: 429") || strings.Contains(msg, "TOOMANYREQUESTS") {
		return ErrorTypeRateLimited
	}
	return ErrorTypeGeneral
}

// isRegistryRateLimited 检查某个 registry 是否仍处于限流状态
func (c *Client) isRegistryRateLimited(registryKey string) bool {
	c.rateLimitMu.RLock()
	defer c.rateLimitMu.RUnlock()
	if state, ok := c.rateLimits[registryKey]; ok {
		return time.Now().Before(state.Until)
	}
	return false
}

// setRegistryRateLimited 标记某个 registry 进入限流冷却期
func (c *Client) setRegistryRateLimited(registryKey string, cooldown time.Duration) {
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()
	c.rateLimits[registryKey] = rateLimitState{Until: time.Now().Add(cooldown)}
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

func normalizeImageRef(ref string) (normalized, host, repo, tag string, err error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", "", "", "", err
	}
	named = reference.TagNameOnly(named)
	// tag = reference.FamiliarString(named)
	// FamiliarString returns repo:tag but not host, so rebuild
	name := named.Name() // includes host
	// split host from path
	parts := strings.SplitN(name, "/", 2)
	host = parts[0]
	path := ""
	if len(parts) == 2 {
		path = parts[1]
	}
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}

	// extract tag value
	t, ok := named.(reference.NamedTagged)
	if !ok {
		// default tag latest
		tag = "latest"
	} else {
		tag = t.Tag()
	}
	normalized = fmt.Sprintf("%s/%s:%s", host, path, tag)
	return normalized, host, path, tag, nil
}

// UpdateManifestCredentials 从全局配置更新 manifestClient 的认证凭据
func (c *Client) UpdateManifestCredentials() {
	if c.manifestClient == nil {
		return
	}

	cfg := config.Get()
	if cfg == nil {
		return
	}

	// 清空现有凭据（通过重新设置）
	for _, auth := range cfg.Registry.Auth {
		registryKey := mapHostToRegistryKey(auth.Host)
		if registryKey != "" && auth.Username != "" {
			c.manifestClient.AddCredential(registryKey, auth.Username, auth.Token)
			logger.Logger.Debug("已更新 manifest 客户端凭据",
				zap.String("registry", registryKey),
				zap.String("username", auth.Username))
		}
	}
}

// mapHostToRegistryKey 将 host 配置映射到 manifest 库的 registry key
func mapHostToRegistryKey(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "docker.io", "dockerhub", "registry-1.docker.io", "index.docker.io":
		return manifestpkg.DockerHubKey
	case "ghcr.io":
		return manifestpkg.GHCRKey
	default:
		// 自定义 registry 暂不支持批量模式
		return ""
	}
}

// DigestResult 批量查询的单个结果
type DigestResult struct {
	IndexDigest string
	ChildDigest string
	Error       error
	ErrType     ErrorType
}

// GetRemoteDigestsBatch 批量获取多个镜像的远程 digest
// 使用 manifest 库的批量模式，支持批量认证和并发查询
func (c *Client) GetRemoteDigestsBatch(ctx context.Context, imageRefs []string, isUserCache bool, concurrency int) map[string]DigestResult {
	results := make(map[string]DigestResult)

	if len(imageRefs) == 0 {
		return results
	}

	// 1. 检查缓存，收集需要查询的镜像
	needQuery := make([]string, 0, len(imageRefs))
	imageSpecs := make([]manifestpkg.ImageSpec, 0, len(imageRefs))
	imageRefMap := make(map[string]string) // normalized -> original

	for _, imageRef := range imageRefs {
		normalized, _, _, tag, err := normalizeImageRef(imageRef)
		if err != nil {
			results[imageRef] = DigestResult{Error: err, ErrType: ErrorTypeGeneral}
			continue
		}

		imageRefMap[normalized] = imageRef

		// 检查缓存（含负缓存）
		if isUserCache {
			if entry, ok := c.getCache(normalized); ok {
				if entry.ErrorType != ErrorTypeNone {
					results[imageRef] = DigestResult{
						Error:   fmt.Errorf("cached error: %s", entry.ErrorType),
						ErrType: entry.ErrorType,
					}
				} else {
					results[imageRef] = DigestResult{IndexDigest: entry.Digest, ChildDigest: ""}
				}
				continue
			}
		}

		// 检查该镜像所在 registry 是否被限流
		imageName := strings.Split(imageRef, ":")[0]
		registryKey := manifestpkg.DetectRegistry(imageName)
		if c.isRegistryRateLimited(registryKey) {
			results[imageRef] = DigestResult{
				Error:   fmt.Errorf("registry %s 请求频率超限，等待冷却", registryKey),
				ErrType: ErrorTypeRateLimited,
			}
			continue
		}

		// 需要查询
		needQuery = append(needQuery, imageRef)

		imageSpecs = append(imageSpecs, manifestpkg.ImageSpec{
			Image: imageName,
			Tag:   tag,
		})
	}

	// 2. 如果没有需要查询的，直接返回
	if len(needQuery) == 0 {
		return results
	}

	// 2.5 优先通过用户配置的 Docker Hub mirror 查询 docker.io 镜像
	// 命中的镜像会从 needQuery / imageSpecs 中剔除，剩余镜像继续走 manifestpkg 流程
	mirrors := dockercli.EnabledMirrorHosts()
	if len(mirrors) > 0 {
		ttlForMirror := time.Minute * 5
		if cfgTmp := config.Get(); cfgTmp != nil && cfgTmp.Scan.CacheTTL > 0 {
			ttlForMirror = cfgTmp.Scan.CacheTTL.Duration()
		}

		filteredQuery := needQuery[:0]
		filteredSpecs := imageSpecs[:0]
		for i, imageRef := range needQuery {
			if !dockercli.IsDockerHubImage(imageRef) {
				filteredQuery = append(filteredQuery, imageRef)
				filteredSpecs = append(filteredSpecs, imageSpecs[i])
				continue
			}
			body, digest, ok := c.queryDigestViaMirrors(imageRef, mirrors)
			if !ok {
				filteredQuery = append(filteredQuery, imageRef)
				filteredSpecs = append(filteredSpecs, imageSpecs[i])
				continue
			}

			childDigest, _ := parseManifest(body)
			results[imageRef] = DigestResult{
				IndexDigest: digest,
				ChildDigest: childDigest,
			}
			if normalized, _, _, _, nerr := normalizeImageRef(imageRef); nerr == nil && normalized != "" {
				c.setCache(normalized, digest, ttlForMirror, ErrorTypeNone)
			}
			logger.Logger.Info("mirror 命中，跳过官方查询",
				zap.String("image", imageRef),
				zap.String("digest", digest))
		}
		needQuery = filteredQuery
		imageSpecs = filteredSpecs
	}

	// 全部命中 mirror 时直接返回
	if len(needQuery) == 0 {
		return results
	}

	// 3. 使用 manifestClient 批量查询
	cfg := config.Get()
	if concurrency <= 0 {
		concurrency = 3
	}
	if concurrency > 64 {
		concurrency = 64
	}
	if cfg != nil && cfg.Scan.Concurrency > 0 {
		concurrency = cfg.Scan.Concurrency
	}

	logger.Logger.Debug("批量获取 manifest",
		zap.Int("total", len(imageSpecs)),
		zap.Int("concurrency", concurrency))

	manifestResults := c.manifestClient.GetManifestsWithDigest(imageSpecs, concurrency, true, nil)

	// 4. 解析结果并缓存
	ttl := time.Minute * 5
	if cfg != nil && cfg.Scan.CacheTTL > 0 {
		ttl = cfg.Scan.CacheTTL.Duration()
	}

	rateLimitCooldown := 5 * time.Minute
	notFoundCacheTTL := 30 * time.Minute
	rateLimitedRegistries := make(map[string]bool)

	for i, manifestResult := range manifestResults {
		if i >= len(needQuery) {
			break
		}

		imageRef := needQuery[i]

		if manifestResult.Error != nil {
			errType := detectErrorType(manifestResult.Error)

			switch errType {
			case ErrorTypeRateLimited:
				imageName := strings.Split(imageRef, ":")[0]
				registryKey := manifestpkg.DetectRegistry(imageName)
				if !rateLimitedRegistries[registryKey] {
					rateLimitedRegistries[registryKey] = true
					c.setRegistryRateLimited(registryKey, rateLimitCooldown)
				}
				results[imageRef] = DigestResult{Error: manifestResult.Error, ErrType: ErrorTypeRateLimited}
				logger.Logger.Warn("镜像请求频率超限",
					zap.String("image", imageRef))

			case ErrorTypeNotFound:
				normalized, _, _, _, _ := normalizeImageRef(imageRef)
				if normalized != "" {
					c.setCache(normalized, "", notFoundCacheTTL, ErrorTypeNotFound)
				}
				results[imageRef] = DigestResult{Error: manifestResult.Error, ErrType: ErrorTypeNotFound}
				logger.Logger.Warn("镜像或标签不存在，已缓存以避免重复查询",
					zap.String("image", imageRef),
					zap.Duration("cacheTTL", notFoundCacheTTL))

			default:
				results[imageRef] = DigestResult{Error: manifestResult.Error, ErrType: ErrorTypeGeneral}
				logger.Logger.Error("批量获取 manifest 失败",
					zap.String("image", imageRef),
					zap.Error(manifestResult.Error))
			}
			continue
		}

		digest := manifestResult.Digest
		if digest == "" {
			results[imageRef] = DigestResult{Error: fmt.Errorf("empty digest"), ErrType: ErrorTypeGeneral}
			continue
		}

		// 缓存结果
		normalized, _, _, _, _ := normalizeImageRef(imageRef)
		if normalized != "" {
			c.setCache(normalized, digest, ttl, ErrorTypeNone)
		}

		manifest := manifestResult.Manifest
		childDigest, _ := parseManifest(manifest)
		results[imageRef] = DigestResult{
			IndexDigest: digest,
			ChildDigest: childDigest,
		}

		logger.Logger.Debug("批量获取 manifest 成功",
			zap.String("image", imageRef),
			zap.String("digest", digest))
	}

	// 对于 429 之后还未处理的镜像，直接标记为限流
	for i := len(manifestResults); i < len(needQuery); i++ {
		imageRef := needQuery[i]
		if _, exists := results[imageRef]; !exists {
			results[imageRef] = DigestResult{
				Error:   fmt.Errorf("跳过查询：registry 请求频率超限"),
				ErrType: ErrorTypeRateLimited,
			}
		}
	}

	return results
}

func parseManifest(manifest string) (childDigest string, err error) {
	var idx v1.Index
	if err := json.Unmarshal([]byte(manifest), &idx); err != nil {
		return "", fmt.Errorf("decode index: %w", err)
	}
	digest, derr := selectDigestFromIndex(idx)
	if derr != nil {
		return "", derr
	}
	return digest, nil
}

func selectDigestFromIndex(idx v1.Index) (string, error) {
	os := runtime.GOOS
	arch := runtime.GOARCH
	for _, m := range idx.Manifests {
		if m.Platform == nil {
			continue
		}
		if strings.EqualFold(m.Platform.OS, os) && strings.EqualFold(m.Platform.Architecture, arch) {
			return m.Digest.String(), nil
		}
	}
	// fallback first digest
	if len(idx.Manifests) > 0 {
		return idx.Manifests[0].Digest.String(), nil
	}
	return "", fmt.Errorf("no manifests in index")
}
