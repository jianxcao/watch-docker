package registry

import (
	"bytes"
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

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

const maxManifestBytes = 4 << 20

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

// fetchManifestFromMirror 通过指定的 mirror 端点查询 docker.io 镜像的 manifest 与 digest
func (c *Client) fetchManifestFromMirror(ctx context.Context, ep dockercli.MirrorEndpoint, imageRef string) (manifest []byte, digest string, bearer string, mirrorRepoPath string, err error) {
	if ep.Host == "" {
		return nil, "", "", "", fmt.Errorf("mirror host 为空")
	}

	_, _, rawRepoPath, tag, err := normalizeImageRef(imageRef)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("解析镜像引用失败: %w", err)
	}
	if rawRepoPath == "" {
		return nil, "", "", "", fmt.Errorf("无法提取镜像路径: %s", imageRef)
	}

	mirrorRepoPath = ep.RepoPath(rawRepoPath)
	mirrorBase := ep.BaseURL()
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", mirrorBase, mirrorRepoPath, tag)

	body, headers, status, err := c.requestManifest(ctx, manifestURL, "")
	if err != nil {
		return nil, "", "", "", err
	}

	if status == http.StatusUnauthorized {
		wwwAuth := headers.Get("Www-Authenticate")
		if wwwAuth == "" {
			return nil, "", "", "", fmt.Errorf("mirror %s 返回 401 但未携带 Www-Authenticate", ep.HostWithPrefix())
		}
		token, terr := c.acquireMirrorToken(ctx, wwwAuth, mirrorRepoPath)
		if terr != nil {
			return nil, "", "", "", fmt.Errorf("mirror %s 认证失败: %w", ep.HostWithPrefix(), terr)
		}
		bearer = token
		body, headers, status, err = c.requestManifest(ctx, manifestURL, bearer)
		if err != nil {
			return nil, "", "", "", err
		}
	}

	if status != http.StatusOK {
		return nil, "", "", "", fmt.Errorf("获取 manifest 失败 (状态码: %d): %s", status, truncate(string(body), 200))
	}

	sum := sha256.Sum256(body)
	bodyDigest := "sha256:" + hex.EncodeToString(sum[:])
	if hdrDigest := strings.TrimSpace(headers.Get("Docker-Content-Digest")); hdrDigest != "" && !strings.EqualFold(hdrDigest, bodyDigest) {
		return nil, "", "", "", fmt.Errorf("mirror %s 返回的 Docker-Content-Digest (%s) 与响应体校验和不匹配 (%s)", ep.HostWithPrefix(), hdrDigest, bodyDigest)
	}
	digest = bodyDigest
	return body, digest, bearer, mirrorRepoPath, nil
}

func (c *Client) fetchChildManifestConfigDigest(ctx context.Context, ep dockercli.MirrorEndpoint, mirrorRepoPath, childDigest, bearer string) string {
	if childDigest == "" {
		return ""
	}
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", ep.BaseURL(), mirrorRepoPath, childDigest)
	body, headers, status, err := c.requestManifest(ctx, manifestURL, bearer)
	if err == nil && status == http.StatusUnauthorized {
		if wwwAuth := headers.Get("Www-Authenticate"); wwwAuth != "" {
			if tok, terr := c.acquireMirrorToken(ctx, wwwAuth, mirrorRepoPath); terr == nil {
				body, _, status, err = c.requestManifest(ctx, manifestURL, tok)
			}
		}
	}
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

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
		if err != nil {
			return nil, resp.Header, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
		}
		if len(body) > maxManifestBytes {
			return nil, resp.Header, resp.StatusCode, fmt.Errorf("manifest 大小超过上限 (%d bytes)", maxManifestBytes)
		}
		return body, resp.Header, resp.StatusCode, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
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
	realmURL, err := url.Parse(realm)
	if err != nil || realmURL == nil || realmURL.Host == "" ||
		(!strings.EqualFold(realmURL.Scheme, "https") && !strings.EqualFold(realmURL.Scheme, "http")) {
		return "", fmt.Errorf("非法的认证 realm: %s", realm)
	}
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", repoPath)
	}

	q := realmURL.Query()
	if service != "" {
		q.Set("service", service)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	realmURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realmURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("创建认证请求失败: %w", err)
	}

	// 仅向官方 auth.docker.io HTTPS 端点发送 Docker Hub 凭据；其余仅在显式匹配该 host 且安全传输时发送
	if strings.EqualFold(realmURL.Scheme, "https") && strings.EqualFold(realmURL.Hostname(), "auth.docker.io") {
		if cred, ok := c.getCredentialForHost("index.docker.io"); ok && cred.Username != "" && cred.Password != "" {
			req.SetBasicAuth(cred.Username, cred.Password)
		}
	} else if cred, ok := c.getCredentialForHost(realmURL.Host); ok && cred.Username != "" && cred.Password != "" {
		if strings.EqualFold(realmURL.Scheme, "https") || c.isHostExplicitHTTP(realmURL.Host) {
			req.SetBasicAuth(cred.Username, cred.Password)
		}
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

func (c *Client) queryDigestViaMirrors(ctx context.Context, imageRef string, mirrors []dockercli.MirrorEndpoint, targetOS, targetArch, targetVariant string, needConfig bool) (indexDigest, childDigest, configDigest string, ok bool) {
	for _, ep := range mirrors {
		body, dig, bearer, mirrorRepoPath, err := c.fetchManifestFromMirror(ctx, ep, imageRef)
		if err != nil || dig == "" {
			if err != nil {
				logger.Logger.Debug("mirror 获取 digest 失败，尝试下一个",
					zap.String("image", imageRef),
					zap.String("mirror", ep.HostWithPrefix()),
					zap.Error(err))
			}
			continue
		}

		trimmed := bytes.TrimSpace(body)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			logger.Logger.Debug("mirror 返回非 JSON 对象 manifest，尝试下一个",
				zap.String("image", imageRef),
				zap.String("mirror", ep.HostWithPrefix()))
			continue
		}

		var m struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
			Manifests []json.RawMessage `json:"manifests"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			logger.Logger.Debug("mirror 返回损坏的 manifest JSON，尝试下一个",
				zap.String("image", imageRef),
				zap.String("mirror", ep.HostWithPrefix()),
				zap.Error(err))
			continue
		}

		if len(m.Manifests) > 0 {
			var im v1.IndexManifest
			if err := json.Unmarshal(body, &im); err != nil {
				logger.Logger.Debug("mirror 返回损坏的 index manifest JSON，尝试下一个",
					zap.String("image", imageRef),
					zap.String("mirror", ep.HostWithPrefix()),
					zap.Error(err))
				continue
			}
			childDig, _ := parseManifest(body, targetOS, targetArch, targetVariant)
			var cfgDig string
			if needConfig {
				if childDig != "" && mirrorRepoPath != "" {
					cfgDig = c.fetchChildManifestConfigDigest(ctx, ep, mirrorRepoPath, childDig, bearer)
				}
				if cfgDig == "" {
					logger.Logger.Debug("mirror 未能获取到子 manifest config digest，尝试下一个或回退",
						zap.String("image", imageRef),
						zap.String("mirror", ep.HostWithPrefix()))
					continue
				}
			}
			logger.Logger.Debug("通过 mirror 获取 digest 成功",
				zap.String("image", imageRef),
				zap.String("mirror", ep.HostWithPrefix()),
				zap.String("digest", dig))
			return dig, childDig, cfgDig, true
		}

		if needConfig && m.Config.Digest == "" {
			logger.Logger.Debug("mirror manifest 缺少 config digest，尝试下一个或回退",
				zap.String("image", imageRef),
				zap.String("mirror", ep.HostWithPrefix()))
			continue
		}
		return dig, dig, m.Config.Digest, true
	}
	return "", "", "", false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
