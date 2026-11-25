package dockercli

import (
	"net/url"
	"strings"

	"github.com/distribution/reference"
	"github.com/jianxcao/watch-docker/backend/internal/config"
)

// MirrorEndpoint 表示规范化后的 mirror 服务端点（保留 http/https 协议与可选路径前缀）。
type MirrorEndpoint struct {
	Scheme     string
	Host       string
	PathPrefix string
}

func (e MirrorEndpoint) BaseURL() string {
	return e.Scheme + "://" + e.Host
}

func (e MirrorEndpoint) HostWithPrefix() string {
	if e.PathPrefix != "" {
		return e.Host + "/" + e.PathPrefix
	}
	return e.Host
}

func (e MirrorEndpoint) RepoPath(repo string) string {
	trimmed := strings.TrimPrefix(repo, "/")
	if e.PathPrefix != "" {
		return e.PathPrefix + "/" + trimmed
	}
	return trimmed
}

// ParseMirrorEndpoint 解析 mirror URL，保留 http/https 协议与路径前缀。
func ParseMirrorEndpoint(rawURL string) (MirrorEndpoint, bool) {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return MirrorEndpoint{}, false
	}
	scheme := "https"
	if strings.HasPrefix(strings.ToLower(s), "http://") {
		scheme = "http"
	}
	if !strings.Contains(s, "://") {
		s = scheme + "://" + strings.TrimPrefix(s, "//")
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return MirrorEndpoint{}, false
	}
	if u.Scheme != "" {
		scheme = strings.ToLower(u.Scheme)
	}
	return MirrorEndpoint{
		Scheme:     scheme,
		Host:       u.Host,
		PathPrefix: strings.Trim(u.Path, "/"),
	}, true
}

// EnabledMirrorEndpoints 返回当前启用的 Docker Hub mirror 端点列表（按配置顺序）。
func EnabledMirrorEndpoints() []MirrorEndpoint {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	endpoints := make([]MirrorEndpoint, 0, len(cfg.Registry.Mirrors))
	for _, m := range cfg.Registry.Mirrors {
		if !m.Enabled {
			continue
		}
		if ep, ok := ParseMirrorEndpoint(m.URL); ok {
			endpoints = append(endpoints, ep)
		}
	}
	return endpoints
}

// EnabledMirrorHosts 返回当前启用的 Docker Hub mirror 主机列表（已规范化、按数组顺序）。
// 仅返回 enabled=true 且 url 非空的条目。
func EnabledMirrorHosts() []string {
	endpoints := EnabledMirrorEndpoints()
	hosts := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		hosts = append(hosts, ep.HostWithPrefix())
	}
	return hosts
}

// IsDockerHubImage 判断给定镜像引用是否属于 Docker Hub (docker.io)。
// 例如：
//   - "nginx" / "nginx:latest" / "library/nginx" / "org/repo:tag" → true
//   - "docker.io/nginx" / "index.docker.io/library/nginx" → true
//   - "ghcr.io/foo/bar" / "registry.example.com/foo" / "localhost:5000/foo" → false
func IsDockerHubImage(ref string) bool {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return false
	}
	named, err := reference.ParseNormalizedNamed(trimmed)
	if err != nil {
		return false
	}
	domain := strings.ToLower(reference.Domain(named))
	return domain == "docker.io" || domain == "index.docker.io" || domain == "registry-1.docker.io"
}

// NormalizeMirrorURL 规范化 mirror URL，返回不含协议、去掉尾部斜杠的 host(+path) 部分。
// 例如：
//   - "https://docker.m.daocloud.io" → "docker.m.daocloud.io"
//   - "https://mirror.example.com/dockerhub/" → "mirror.example.com/dockerhub"
//   - "docker.m.daocloud.io" → "docker.m.daocloud.io"
func NormalizeMirrorURL(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return ""
	}
	// 去掉协议前缀
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		host := u.Host
		path := strings.Trim(u.Path, "/")
		if path != "" {
			return host + "/" + path
		}
		return host
	}
	// 没有协议时直接处理
	s = strings.TrimPrefix(s, "//")
	s = strings.TrimRight(s, "/")
	return s
}

// RewriteRefToMirror 将 docker.io 镜像引用重写为通过 mirror 的引用。
// mirrorHost 必须已经经过 NormalizeMirrorURL 处理（不含协议）。
// 非 docker.io 镜像或解析失败时，返回原 ref。
//
// 示例（mirrorHost = "docker.m.daocloud.io"）：
//   - "nginx"                       → "docker.m.daocloud.io/library/nginx:latest"
//   - "nginx:1.25"                  → "docker.m.daocloud.io/library/nginx:1.25"
//   - "library/nginx:1.25"          → "docker.m.daocloud.io/library/nginx:1.25"
//   - "bitnami/redis:7"             → "docker.m.daocloud.io/bitnami/redis:7"
//   - "nginx@sha256:abc..."         → "docker.m.daocloud.io/library/nginx@sha256:abc..."
//   - "ghcr.io/foo/bar:tag"         → 原样返回
func RewriteRefToMirror(ref string, mirrorHost string) string {
	trimmed := strings.TrimSpace(ref)
	if mirrorHost == "" || trimmed == "" {
		return ref
	}
	if !IsDockerHubImage(trimmed) {
		return ref
	}

	named, err := reference.ParseNormalizedNamed(trimmed)
	if err != nil {
		return ref
	}

	// reference.Path 返回不含 host 的镜像路径（如 "library/nginx"）
	path := reference.Path(named)
	suffix := ""

	// 优先 digest
	if d, ok := named.(reference.Digested); ok {
		suffix = "@" + d.Digest().String()
	} else if t, ok := named.(reference.Tagged); ok {
		suffix = ":" + t.Tag()
	} else {
		// 默认 latest
		suffix = ":latest"
	}

	return mirrorHost + "/" + path + suffix
}

// NormalizeRef 把镜像引用规范化为含 tag/digest 的标准形式。
// 用于 retag 时确定 target ref（保证和原始 ref 一致，避免 daemon 处理不一致）。
//
// 示例：
//   - "nginx"            → "docker.io/library/nginx:latest"
//   - "nginx:1.25"       → "docker.io/library/nginx:1.25"
//   - "ghcr.io/foo:tag"  → "ghcr.io/foo:tag"
func NormalizeRef(ref string) string {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	named = reference.TagNameOnly(named)
	return named.String()
}
