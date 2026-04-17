package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"

	manifestpkg "github.com/docker-make/docker-mainifest/pkg/registry"
)

// mirrorManifestAccept 标准 manifest Accept 头集合，覆盖 v2 / index / oci 等格式。
var mirrorManifestAccept = []string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.image.index.v1+json",
}

// mirrorTokenResponse 与 manifestpkg.tokenResponse 等价的私有副本。
type mirrorTokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// fetchManifestFromMirror 通过指定的 mirror 主机查询 docker.io 镜像的 manifest 与 digest。
//
// mirrorHost: 已规范化的 mirror 主机（不含协议），例如 "docker.m.daocloud.io"
// imageRef:   原始 docker.io 镜像引用，例如 "nginx:1.25" / "bitnami/redis:7"
//
// 实现：先匿名请求 mirror 的 /v2/{path}/manifests/{tag}；若返回 401 则解析
// WWW-Authenticate（一般指向 docker.io 官方认证服务）拿 bearer token 后再次请求。
// 成功则优先从 Docker-Content-Digest 响应头取 digest，缺省则对响应体计算 sha256。
func (c *Client) fetchManifestFromMirror(mirrorHost, imageRef string) (manifest string, digest string, err error) {
	if mirrorHost == "" {
		return "", "", fmt.Errorf("mirror host 为空")
	}

	_, _, repoPath, tag, err := normalizeImageRef(imageRef)
	if err != nil {
		return "", "", fmt.Errorf("解析镜像引用失败: %w", err)
	}
	// normalizeImageRef 已经把 docker.io 单段镜像补全为 library/xxx
	if repoPath == "" {
		return "", "", fmt.Errorf("无法提取镜像路径: %s", imageRef)
	}

	mirrorBase := mirrorBaseURL(mirrorHost)
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", mirrorBase, repoPath, tag)

	// 第一次尝试：匿名
	body, headers, status, err := c.requestManifest(manifestURL, "")
	if err != nil {
		return "", "", err
	}

	if status == http.StatusUnauthorized {
		wwwAuth := headers.Get("Www-Authenticate")
		if wwwAuth == "" {
			return "", "", fmt.Errorf("mirror %s 返回 401 但未携带 Www-Authenticate", mirrorHost)
		}
		token, terr := c.acquireMirrorToken(wwwAuth, repoPath)
		if terr != nil {
			return "", "", fmt.Errorf("mirror %s 认证失败: %w", mirrorHost, terr)
		}
		body, headers, status, err = c.requestManifest(manifestURL, token)
		if err != nil {
			return "", "", err
		}
	}

	if status != http.StatusOK {
		return "", "", fmt.Errorf("获取 manifest 失败 (状态码: %d): %s", status, truncate(string(body), 200))
	}

	digest = headers.Get("Docker-Content-Digest")
	if digest == "" {
		// 缺省时按响应体计算 sha256（registry 规范允许）
		sum := sha256.Sum256(body)
		digest = "sha256:" + hex.EncodeToString(sum[:])
	}
	return string(body), digest, nil
}

// requestManifest 发起一次带 manifest Accept 头的 GET 请求，返回响应体与状态码。
func (c *Client) requestManifest(manifestURL, bearer string) ([]byte, http.Header, int, error) {
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("创建请求失败: %w", err)
	}
	for _, a := range mirrorManifestAccept {
		req.Header.Add("Accept", a)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	httpClient := c.mirrorHTTPClient()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
	}
	return body, resp.Header, resp.StatusCode, nil
}

// acquireMirrorToken 通过 WWW-Authenticate 提供的 realm/service/scope 获取 token。
// 若解析出的 scope 缺省，则使用 repository:{path}:pull 兜底。
func (c *Client) acquireMirrorToken(wwwAuth, repoPath string) (string, error) {
	realm, service, scope, err := manifestpkg.ParseWWWAuthenticate(wwwAuth)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", repoPath)
	}

	authURL := realm
	q := url.Values{}
	if service != "" {
		q.Set("service", service)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	if len(q) > 0 {
		if strings.Contains(authURL, "?") {
			authURL += "&" + q.Encode()
		} else {
			authURL += "?" + q.Encode()
		}
	}

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建认证请求失败: %w", err)
	}

	// 如果配置了 docker.io 的凭据，带上 Basic Auth（部分镜像需要登录）
	if cred, ok := c.manifestClient.GetCredential(manifestpkg.DockerHubKey); ok && cred.Username != "" && cred.Token != "" {
		req.SetBasicAuth(cred.Username, cred.Token)
	}

	resp, err := c.mirrorHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("认证请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("认证失败 (状态码: %d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var tk mirrorTokenResponse
	if err := json.Unmarshal(body, &tk); err != nil {
		return "", fmt.Errorf("解析认证响应失败: %w", err)
	}
	if tk.Token != "" {
		return tk.Token, nil
	}
	if tk.AccessToken != "" {
		return tk.AccessToken, nil
	}
	return "", fmt.Errorf("认证响应中没有 token")
}

// mirrorHTTPClient 返回带超时的 http 客户端，遵循全局代理配置。
func (c *Client) mirrorHTTPClient() *http.Client {
	cfg := config.Get()
	transport := &http.Transport{}
	if cfg != nil && strings.TrimSpace(cfg.Proxy.URL) != "" {
		if proxyURL, err := url.Parse(cfg.Proxy.URL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}

// queryDigestViaMirrors 按顺序尝试每个 mirror，第一个返回成功的即采用。
// 仅当 imageRef 是 docker.io 镜像且配置了启用的 mirror 时才会被调用。
func (c *Client) queryDigestViaMirrors(imageRef string, mirrors []string) (manifest string, digest string, ok bool) {
	for _, host := range mirrors {
		body, dig, err := c.fetchManifestFromMirror(host, imageRef)
		if err == nil && dig != "" {
			logger.Logger.Debug("通过 mirror 获取 digest 成功",
				zap.String("image", imageRef),
				zap.String("mirror", host),
				zap.String("digest", dig))
			return body, dig, true
		}
		if err != nil {
			logger.Logger.Debug("mirror 获取 digest 失败，尝试下一个",
				zap.String("image", imageRef),
				zap.String("mirror", host),
				zap.Error(err))
		}
	}
	return "", "", false
}

func mirrorBaseURL(host string) string {
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return strings.TrimRight(host, "/")
	}
	return "https://" + strings.TrimRight(host, "/")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
