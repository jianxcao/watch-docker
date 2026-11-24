package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

var mirrorManifestAccept = []string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.image.index.v1+json",
}

type mirrorTokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

var wwwAuthParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func parseWWWAuthenticate(header string) (realm, service, scope string, err error) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("empty Www-Authenticate header")
	}
	matches := wwwAuthParamRe.FindAllStringSubmatch(trimmed, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		switch strings.ToLower(m[1]) {
		case "realm":
			realm = m[2]
		case "service":
			service = m[2]
		case "scope":
			scope = m[2]
		}
	}
	if realm == "" {
		return "", "", "", fmt.Errorf("missing realm in Www-Authenticate: %s", header)
	}
	return realm, service, scope, nil
}

func (c *Client) mirrorHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: c.getTransport(),
	}
}

// fetchManifestFromMirror 通过指定的 mirror 主机查询 docker.io 镜像的 manifest 与 digest
func (c *Client) fetchManifestFromMirror(ctx context.Context, mirrorHost, imageRef string) (manifest []byte, digest string, bearer string, err error) {
	if mirrorHost == "" {
		return nil, "", "", fmt.Errorf("mirror host 为空")
	}

	_, _, repoPath, tag, err := normalizeImageRef(imageRef)
	if err != nil {
		return nil, "", "", fmt.Errorf("解析镜像引用失败: %w", err)
	}
	if repoPath == "" {
		return nil, "", "", fmt.Errorf("无法提取镜像路径: %s", imageRef)
	}

	mirrorBase := mirrorBaseURL(mirrorHost)
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", mirrorBase, repoPath, tag)

	body, headers, status, err := c.requestManifest(ctx, manifestURL, "")
	if err != nil {
		return nil, "", "", err
	}

	if status == http.StatusUnauthorized {
		wwwAuth := headers.Get("Www-Authenticate")
		if wwwAuth == "" {
			return nil, "", "", fmt.Errorf("mirror %s 返回 401 但未携带 Www-Authenticate", mirrorHost)
		}
		token, terr := c.acquireMirrorToken(ctx, wwwAuth, repoPath)
		if terr != nil {
			return nil, "", "", fmt.Errorf("mirror %s 认证失败: %w", mirrorHost, terr)
		}
		bearer = token
		body, headers, status, err = c.requestManifest(ctx, manifestURL, bearer)
		if err != nil {
			return nil, "", "", err
		}
	}

	if status != http.StatusOK {
		return nil, "", "", fmt.Errorf("获取 manifest 失败 (状态码: %d): %s", status, truncate(string(body), 200))
	}

	digest = headers.Get("Docker-Content-Digest")
	if digest == "" {
		sum := sha256.Sum256(body)
		digest = "sha256:" + hex.EncodeToString(sum[:])
	}
	return body, digest, bearer, nil
}

func (c *Client) fetchChildManifestConfigDigest(ctx context.Context, mirrorHost, repoPath, childDigest, bearer string) string {
	if childDigest == "" {
		return ""
	}
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", mirrorBaseURL(mirrorHost), repoPath, childDigest)
	body, _, status, err := c.requestManifest(ctx, manifestURL, bearer)
	if err != nil || status != http.StatusOK {
		return ""
	}
	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &m); err == nil {
		return m.Config.Digest
	}
	return ""
}

func (c *Client) requestManifest(ctx context.Context, manifestURL, bearer string) ([]byte, http.Header, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("创建请求失败: %w", err)
	}
	for _, a := range mirrorManifestAccept {
		req.Header.Add("Accept", a)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.mirrorHTTPClient().Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
	}
	return body, resp.Header, resp.StatusCode, nil
}

func (c *Client) acquireMirrorToken(ctx context.Context, wwwAuth, repoPath string) (string, error) {
	realm, service, scope, err := parseWWWAuthenticate(wwwAuth)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建认证请求失败: %w", err)
	}

	if cred, ok := c.getCredentialForHost("index.docker.io"); ok && cred.Username != "" && cred.Password != "" {
		req.SetBasicAuth(cred.Username, cred.Password)
	}

	resp, err := c.mirrorHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("认证请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
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

func (c *Client) queryDigestViaMirrors(ctx context.Context, imageRef string, mirrors []string, targetOS, targetArch, targetVariant string, needConfig bool) (indexDigest, childDigest, configDigest string, ok bool) {
	_, _, repoPath, _, _ := normalizeImageRef(imageRef)
	for _, host := range mirrors {
		body, dig, bearer, err := c.fetchManifestFromMirror(ctx, host, imageRef)
		if err != nil || dig == "" {
			if err != nil {
				logger.Logger.Debug("mirror 获取 digest 失败，尝试下一个",
					zap.String("image", imageRef),
					zap.String("mirror", host),
					zap.Error(err))
			}
			continue
		}

		var m struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
			Manifests []json.RawMessage `json:"manifests"`
		}
		_ = json.Unmarshal(body, &m)

		if len(m.Manifests) > 0 {
			childDig, _ := parseManifest(body, targetOS, targetArch, targetVariant)
			var cfgDig string
			if needConfig && childDig != "" && repoPath != "" {
				cfgDig = c.fetchChildManifestConfigDigest(ctx, host, repoPath, childDig, bearer)
			}
			logger.Logger.Debug("通过 mirror 获取 digest 成功",
				zap.String("image", imageRef),
				zap.String("mirror", host),
				zap.String("digest", dig))
			return dig, childDig, cfgDig, true
		}

		return dig, dig, m.Config.Digest, true
	}
	return "", "", "", false
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
