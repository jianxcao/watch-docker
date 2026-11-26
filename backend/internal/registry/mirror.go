package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
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

func parseBearerFromWWWAuthenticate(header string) (realm, service, scope string, hasBearer, ok bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return "", "", "", false, false
	}

	i := 0
	n := len(trimmed)
	expectChallenge := true
	currentScheme := ""
	curRealm, curService, curScope := "", "", ""

	skipWS := func() {
		for i < n && (trimmed[i] == ' ' || trimmed[i] == '\t') {
			i++
		}
	}

	for i < n {
		skipWS()
		for i < n && trimmed[i] == ',' {
			i++
			skipWS()
			expectChallenge = true
		}
		if i >= n {
			break
		}

		// 读取 token（scheme 名或参数 key，或 token68）
		start := i
		for i < n && trimmed[i] != ' ' && trimmed[i] != '\t' && trimmed[i] != '=' && trimmed[i] != ',' && trimmed[i] != '"' {
			i++
		}
		tok := trimmed[start:i]
		if tok == "" {
			if i < n && trimmed[i] == '"' {
				// 跳过意外引号值
				i++
				for i < n && trimmed[i] != '"' {
					if trimmed[i] == '\\' && i+1 < n {
						i += 2
						continue
					}
					i++
				}
				if i < n {
					i++
				}
			} else {
				i++
			}
			expectChallenge = false
			continue
		}

		skipWS()
		if i < n && trimmed[i] == '=' {
			// key=value 参数
			i++
			skipWS()
			var val string
			if i < n && trimmed[i] == '"' {
				i++
				var b strings.Builder
				for i < n && trimmed[i] != '"' {
					if trimmed[i] == '\\' && i+1 < n {
						b.WriteByte(trimmed[i+1])
						i += 2
						continue
					}
					b.WriteByte(trimmed[i])
					i++
				}
				if i < n && trimmed[i] == '"' {
					i++
				}
				val = b.String()
			} else {
				valStart := i
				for i < n && trimmed[i] != ' ' && trimmed[i] != '\t' && trimmed[i] != ',' {
					i++
				}
				val = trimmed[valStart:i]
			}
			if currentScheme == "bearer" {
				switch strings.ToLower(tok) {
				case "realm":
					curRealm = val
				case "service":
					curService = val
				case "scope":
					curScope = val
				}
			}
			skipWS()
			if i < n && trimmed[i] == ',' {
				i++
				expectChallenge = true
			} else {
				expectChallenge = false
			}
			continue
		}

		// 若非 '=' 且处于新 challenge 边界（或上一个 challenge 刚结束），按 RFC 7235 判定为新 auth-scheme
		if expectChallenge || currentScheme == "" {
			if currentScheme == "bearer" && curRealm != "" {
				return curRealm, curService, curScope, true, true
			}
			currentScheme = strings.ToLower(tok)
			if currentScheme == "bearer" {
				hasBearer = true
			}
			curRealm, curService, curScope = "", "", ""
			expectChallenge = false
			continue
		}

		// 既非 '=' 也非新边界（例如 Basic/Negotiate 等后面的 token68），跳至下一个逗号
		for i < n && trimmed[i] != ',' {
			if trimmed[i] == '"' {
				i++
				for i < n && trimmed[i] != '"' {
					if trimmed[i] == '\\' && i+1 < n {
						i += 2
						continue
					}
					i++
				}
				if i < n {
					i++
				}
				continue
			}
			i++
		}
		if i < n && trimmed[i] == ',' {
			i++
			expectChallenge = true
		}
	}

	if currentScheme == "bearer" && curRealm != "" {
		return curRealm, curService, curScope, true, true
	}
	return "", "", "", hasBearer, false
}

func selectBearerWWWAuthenticate(headers http.Header) string {
	fallbackBearer := ""
	for _, h := range headers.Values("Www-Authenticate") {
		if _, _, _, hasBearer, ok := parseBearerFromWWWAuthenticate(h); ok {
			return strings.TrimSpace(h)
		} else if hasBearer && fallbackBearer == "" {
			fallbackBearer = strings.TrimSpace(h)
		}
	}
	return fallbackBearer
}

func parseWWWAuthenticate(header string) (realm, service, scope string, err error) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("empty Www-Authenticate header")
	}
	realm, service, scope, hasBearer, ok := parseBearerFromWWWAuthenticate(trimmed)
	if !hasBearer {
		return "", "", "", fmt.Errorf("unsupported Www-Authenticate scheme: %s", header)
	}
	if !ok {
		return "", "", "", fmt.Errorf("missing realm in Www-Authenticate: %s", header)
	}
	return realm, service, scope, nil
}

func effectivePort(scheme, port string) string {
	if port != "" {
		return port
	}
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func validateMirrorHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return nil, fmt.Errorf("非法的认证 realm: %s", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "https" && scheme != "http") || u.User != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("非法的认证 realm: %s", raw)
	}
	return u, nil
}

func sameURLOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	sa, sb := strings.ToLower(a.Scheme), strings.ToLower(b.Scheme)
	if (sa != "https" && sa != "http") || sa != sb {
		return false
	}
	if a.User != nil || b.User != nil {
		return false
	}
	ha, hb := strings.ToLower(a.Hostname()), strings.ToLower(b.Hostname())
	if ha == "" || ha != hb {
		return false
	}
	return effectivePort(sa, a.Port()) == effectivePort(sb, b.Port())
}

func sameMirrorOrigin(ep dockercli.MirrorEndpoint, target *url.URL) bool {
	epURL, err := validateMirrorHTTPURL(ep.BaseURL())
	if err != nil {
		return false
	}
	return sameURLOrigin(epURL, target)
}

func (c *Client) credentialForMirrorRealm(ep dockercli.MirrorEndpoint, realm *url.URL) (authn.AuthConfig, bool) {
	if !sameMirrorOrigin(ep, realm) {
		return authn.AuthConfig{}, false
	}
	epURL, err := validateMirrorHTTPURL(ep.BaseURL())
	if err != nil {
		return authn.AuthConfig{}, false
	}
	origin := mirrorOrigin{
		scheme: strings.ToLower(epURL.Scheme),
		host:   strings.ToLower(epURL.Hostname()),
		port:   effectivePort(epURL.Scheme, epURL.Port()),
	}
	if canonicalHost(ep.Host) == "index.docker.io" || canonicalHost(origin.host) == "index.docker.io" || origin.host == "auth.docker.io" {
		return authn.AuthConfig{}, false
	}

	c.credsMu.RLock()
	cred, ok := c.mirrorCredentials[origin]
	c.credsMu.RUnlock()
	if !ok || cred.Username == "" || cred.Password == "" {
		return authn.AuthConfig{}, false
	}
	return cred, true
}

func (c *Client) mirrorHTTPClient(credentialed bool) *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: c.getTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if _, err := validateMirrorHTTPURL(req.URL.String()); err != nil {
				req.Header.Del("Authorization")
				return err
			}
			wasCred := credentialed || (len(via) > 0 && via[0].Header.Get("Authorization") != "")
			if wasCred {
				if len(via) == 0 || !sameURLOrigin(via[0].URL, req.URL) {
					req.Header.Del("Authorization")
					return fmt.Errorf("拒绝跨源重定向携带认证凭据")
				}
				if authHdr := via[0].Header.Get("Authorization"); authHdr != "" {
					req.Header.Set("Authorization", authHdr)
				}
				return nil
			}
			req.Header.Del("Authorization")
			return nil
		},
	}
}

func computeAndVerifyManifestDigest(body []byte, expectedDigest string) (string, error) {
	expected := strings.Trim(strings.TrimSpace(expectedDigest), `"`)
	if expected == "" {
		sum := sha256.Sum256(body)
		return "sha256:" + hex.EncodeToString(sum[:]), nil
	}
	alg, wantHex, ok := strings.Cut(expected, ":")
	if !ok || wantHex == "" {
		return "", fmt.Errorf("无效的 manifest digest 格式: %s", expectedDigest)
	}
	algLower := strings.ToLower(strings.TrimSpace(alg))
	var h hash.Hash
	switch algLower {
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("不支持的 manifest digest 算法: %s", alg)
	}
	_, _ = h.Write(body)
	got := algLower + ":" + hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, algLower+":"+strings.TrimSpace(wantHex)) {
		return "", fmt.Errorf("manifest digest 校验和不匹配: got %s want %s", got, expected)
	}
	return got, nil
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
		wwwAuth := selectBearerWWWAuthenticate(headers)
		if wwwAuth == "" {
			return nil, "", "", "", fmt.Errorf("mirror %s 返回 401 但未携带 Bearer Www-Authenticate", ep.HostWithPrefix())
		}
		token, terr := c.acquireMirrorToken(ctx, ep, wwwAuth, mirrorRepoPath)
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

	if strings.Contains(tag, ":") {
		digest, err = computeAndVerifyManifestDigest(body, tag)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("mirror %s %w", ep.HostWithPrefix(), err)
		}
		if hdr := strings.TrimSpace(headers.Get("Docker-Content-Digest")); hdr != "" {
			if _, herr := computeAndVerifyManifestDigest(body, hdr); herr != nil {
				return nil, "", "", "", fmt.Errorf("mirror %s %w", ep.HostWithPrefix(), herr)
			}
		}
	} else {
		digest, err = computeAndVerifyManifestDigest(body, headers.Get("Docker-Content-Digest"))
		if err != nil {
			return nil, "", "", "", fmt.Errorf("mirror %s %w", ep.HostWithPrefix(), err)
		}
	}
	return body, digest, bearer, mirrorRepoPath, nil
}

func (c *Client) fetchChildManifestConfigDigest(ctx context.Context, ep dockercli.MirrorEndpoint, mirrorRepoPath, childDigest, bearer string) string {
	if childDigest == "" {
		return ""
	}
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", ep.BaseURL(), mirrorRepoPath, childDigest)
	body, headers, status, err := c.requestManifest(ctx, manifestURL, bearer)
	if err == nil && status == http.StatusUnauthorized {
		if wwwAuth := selectBearerWWWAuthenticate(headers); wwwAuth != "" {
			if tok, terr := c.acquireMirrorToken(ctx, ep, wwwAuth, mirrorRepoPath); terr == nil {
				body, headers, status, err = c.requestManifest(ctx, manifestURL, tok)
			}
		}
	}
	if err != nil || status != http.StatusOK {
		return ""
	}
	if _, err := computeAndVerifyManifestDigest(body, childDigest); err != nil {
		return ""
	}
	if hdr := headers.Get("Docker-Content-Digest"); hdr != "" {
		if _, err := computeAndVerifyManifestDigest(body, hdr); err != nil {
			return ""
		}
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

	resp, err := c.mirrorHTTPClient(bearer != "").Do(req)
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

func (c *Client) acquireMirrorToken(ctx context.Context, ep dockercli.MirrorEndpoint, wwwAuth, repoPath string) (string, error) {
	realm, service, scope, err := parseWWWAuthenticate(wwwAuth)
	if err != nil {
		return "", err
	}
	realmURL, err := validateMirrorHTTPURL(realm)
	if err != nil {
		return "", err
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

	cred, hasCred := c.credentialForMirrorRealm(ep, realmURL)
	if hasCred {
		req.SetBasicAuth(cred.Username, cred.Password)
	}

	resp, err := c.mirrorHTTPClient(hasCred).Do(req)
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
			hasValidDesc := false
			for _, desc := range im.Manifests {
				if desc.Digest.Algorithm != "" && desc.Digest.Hex != "" {
					hasValidDesc = true
					break
				}
			}
			if !hasValidDesc {
				logger.Logger.Debug("mirror index manifest 缺少有效子摘要，尝试下一个",
					zap.String("image", imageRef),
					zap.String("mirror", ep.HostWithPrefix()))
				continue
			}
			childDig, err := parseManifest(body, targetOS, targetArch, targetVariant)
			if err != nil || childDig == "" {
				logger.Logger.Debug("mirror index manifest 未找到匹配平台的子摘要，尝试下一个",
					zap.String("image", imageRef),
					zap.String("platform", formatPlatformLabel(targetOS, targetArch, targetVariant)),
					zap.String("mirror", ep.HostWithPrefix()),
					zap.Error(err))
				continue
			}
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

		cfgAlg, cfgHex, cfgOK := strings.Cut(strings.TrimSpace(m.Config.Digest), ":")
		if !cfgOK || cfgAlg == "" || cfgHex == "" {
			logger.Logger.Debug("mirror manifest 缺少有效的 config.digest（可能为非 manifest JSON），尝试下一个或回退",
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
