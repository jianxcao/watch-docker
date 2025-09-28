package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"

	"golang.org/x/sync/singleflight"

	"github.com/distribution/reference"
	"github.com/go-resty/resty/v2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type CacheEntry struct {
	Digest string
	Expiry time.Time
}

type Client struct {
	http  *resty.Client
	cache map[string]CacheEntry
	mu    sync.RWMutex
	sf    singleflight.Group
}

func New() *Client {
	httpClient := resty.New().
		SetHeader("User-Agent", "watch-docker/1.0").
		SetRetryCount(2).
		SetRetryWaitTime(500 * time.Millisecond).
		SetRetryMaxWaitTime(2 * time.Second).
		SetTimeout(20 * time.Minute)

	// 应用代理配置
	if cfg := config.Get(); cfg != nil && cfg.Proxy.URL != "" {
		httpClient.SetProxy(cfg.Proxy.URL)
	}

	return &Client{http: httpClient, cache: make(map[string]CacheEntry)}
}

func (c *Client) GetRemoteDigestByCache(ctx context.Context, imageRef string) (digest string, err error) {
	normalized, _, _, _, err := normalizeImageRef(imageRef)
	if err != nil {
		return "", err
	}
	if d, ok := c.getCache(normalized); ok {
		return d, nil
	}
	return "", nil
}

// GetRemoteDigest resolves the digest for the provided image ref (e.g. nginx:latest).
// It supports manifest lists by selecting the child manifest matching runtime platform.
// GetRemoteDigests 返回镜像的索引(manifest list)层 digest 与子镜像(平台专属 manifest)层 digest。
// 对于非多架构镜像，两者相同。
func (c *Client) GetRemoteDigests(ctx context.Context, imageRef string, isUserCache bool) (indexDigest string, childDigest string, err error) {
	normalized, host, repo, referenceTag, err := normalizeImageRef(imageRef)
	if err != nil {
		return "", "", err
	}

	// cache key uses normalized ref (host/repo:tag)
	if isUserCache {
		if d, ok := c.getCache(normalized); ok {
			// 缓存中仅存 indexDigest，为了兼容旧缓存此处 childDigest 置空
			return d, "", nil
		}
	}

	type res struct {
		idx   string
		child string
	}
	v, err, _ := c.sf.Do(normalized, func() (interface{}, error) {
		endpoint := fmt.Sprintf("https://%s/v2/%s/manifests/%s", host, repo, url.PathEscape(referenceTag))

		req := c.http.R().SetContext(ctx).SetHeader("Accept", strings.Join([]string{
			"application/vnd.docker.distribution.manifest.list.v2+json",
			"application/vnd.docker.distribution.manifest.v2+json",
			"application/vnd.oci.image.index.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
		}, ", "))

		if cfg := config.Get(); cfg != nil {
			for _, a := range cfg.Registry.Auth {
				if normalizeRegistryHost(a.Host) == host && a.Username != "" {
					req.SetBasicAuth(a.Username, a.Password)
					break
				}
			}
		}

		resp, err := req.Get(endpoint)
		if err != nil {
			logger.Logger.Error("get remote digest", logger.ZapField("endpoint", endpoint), zap.Int("StatusCode", resp.StatusCode()), logger.ZapErr(err))
			return nil, err
		}

		if resp.StatusCode() == http.StatusUnauthorized {
			token, terr := c.fetchBearerToken(ctx, host, repo, resp.Header().Get("Www-Authenticate"))
			if terr != nil {
				logger.Logger.Error("get remote digest", logger.ZapField("endpoint", endpoint), zap.Int("StatusCode", resp.StatusCode()), logger.ZapErr(terr))
				return nil, fmt.Errorf("bearer token: %w", terr)
			}
			resp, err = req.SetHeader("Authorization", "Bearer "+token).Get(endpoint)
			if err != nil {
				logger.Logger.Error("get remote digest", logger.ZapField("endpoint", endpoint), zap.Int("StatusCode", resp.StatusCode()), logger.ZapErr(err))
				return nil, err
			}
		}

		if resp.IsError() {
			logger.Logger.Error("get remote digest", logger.ZapField("endpoint", endpoint), zap.Int("StatusCode", resp.StatusCode()), logger.ZapErr(fmt.Errorf("registry error: %s", resp.Status())))
			return nil, fmt.Errorf("registry error: %s", resp.Status())
		}

		ct := resp.Header().Get("Content-Type")
		switch {
		case strings.Contains(ct, "manifest.list") || strings.Contains(ct, "+json") && strings.Contains(string(resp.Body()), "manifests"):
			var idx v1.Index
			if err := json.Unmarshal(resp.Body(), &idx); err != nil {
				return nil, fmt.Errorf("decode index: %w", err)
			}
			digest, derr := selectDigestFromIndex(idx)
			if derr != nil {
				return nil, derr
			}
			indexHeader := resp.Header().Get("Docker-Content-Digest")
			if indexHeader == "" {
				sum := sha256.Sum256(resp.Body())
				indexHeader = "sha256:" + hex.EncodeToString(sum[:])
			}
			ttl := time.Minute * 5
			if cfg := config.Get(); cfg != nil && cfg.Scan.CacheTTL > 0 {
				ttl = cfg.Scan.CacheTTL.Duration()
			}
			c.setCache(normalized, indexHeader, ttl)
			return res{idx: indexHeader, child: digest}, nil
		default:
			d := resp.Header().Get("Docker-Content-Digest")
			if d == "" {
				sum := sha256.Sum256(resp.Body())
				d = "sha256:" + hex.EncodeToString(sum[:])
			}
			ttl := time.Minute * 5
			if cfg := config.Get(); cfg != nil && cfg.Scan.CacheTTL > 0 {
				ttl = cfg.Scan.CacheTTL.Duration()
			}
			c.setCache(normalized, d, ttl)
			return res{idx: d, child: d}, nil
		}
	})
	if err != nil {
		return "", "", err
	}
	rr := v.(res)
	return rr.idx, rr.child, nil
}

func (c *Client) fetchBearerToken(ctx context.Context, host, repo, wwwAuth string) (string, error) {
	// Example: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"
	if !strings.HasPrefix(strings.ToLower(wwwAuth), "bearer ") {
		return "", fmt.Errorf("unsupported auth: %s", wwwAuth)
	}
	params := parseAuthParams(strings.TrimSpace(wwwAuth[len("bearer "):]))
	realm := params["realm"]
	service := params["service"]
	scope := params["scope"]
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", repo)
	}

	u, err := url.Parse(realm)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if service != "" {
		q.Set("service", service)
	}
	q.Set("scope", scope)
	u.RawQuery = q.Encode()

	req := c.http.R().SetContext(ctx)
	// basic auth may be required for private registries token endpoint (dynamic from config)
	if cfg := config.Get(); cfg != nil {
		for _, a := range cfg.Registry.Auth {
			if normalizeRegistryHost(a.Host) == host && a.Username != "" {
				req.SetBasicAuth(a.Username, a.Password)
				break
			}
		}
	}
	logger.Logger.Debug("获取 bearer token", logger.ZapField("url", u.String()))
	tr, err := req.Get(u.String())
	if err != nil {
		return "", err
	}
	if tr.IsError() {
		return "", fmt.Errorf("token endpoint error: %s", tr.Status())
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(tr.Body(), &tok); err != nil {
		return "", err
	}
	if tok.Token == "" {
		return "", fmt.Errorf("empty token")
	}
	return tok.Token, nil
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

func (c *Client) getCache(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.Expiry) {
		return e.Digest, true
	}
	return "", false
}

func (c *Client) setCache(key, digest string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = CacheEntry{Digest: digest, Expiry: time.Now().Add(ttl)}
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

func normalizeRegistryHost(h string) string {
	switch h {
	case "docker.io", "index.docker.io":
		return "registry-1.docker.io"
	default:
		return h
	}
}

func parseAuthParams(s string) map[string]string {
	res := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(kv[0]))
		val := strings.Trim(kv[1], "\"")
		res[key] = val
	}
	return res
}
