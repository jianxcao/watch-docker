package dockercli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/distribution/reference"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/image"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"
)

// ImageInfo 镜像基础信息（用于列表展示）
type ImageInfo struct {
	ID          string   `json:"id"`
	RepoTags    []string `json:"repoTags"`
	RepoDigests []string `json:"repoDigests"`
	Size        int64    `json:"size"`
	Created     int64    `json:"created"`
}

// PullProgress Docker pull 进度事件
type PullProgress struct {
	Status         string           `json:"status"`
	ID             string           `json:"id"`
	Progress       string           `json:"progress"`
	ProgressDetail *ProgressDetail  `json:"progressDetail,omitempty"`
	Error          string           `json:"error,omitempty"`
	ErrorDetail    *PullErrorDetail `json:"errorDetail,omitempty"`
}

type ProgressDetail struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

type PullErrorDetail struct {
	Message string `json:"message"`
}

// ImagePull 拉取镜像（支持私有仓库认证与 Docker Hub Mirror 自动回退）。
func (c *Client) ImagePull(ctx context.Context, ref string) error {
	return c.pullWithMirrorFallback(ctx, ref, nil, nil)
}

// ImagePullWithProgress 拉取镜像并通过回调报告进度。
func (c *Client) ImagePullWithProgress(ctx context.Context, ref string, onProgress func(PullProgress)) error {
	return c.pullWithMirrorFallback(ctx, ref, nil, onProgress)
}

// ImagePullWithPlatform 按目标平台拉取镜像（支持非宿主机原生架构容器更新与进度回调）。
func (c *Client) ImagePullWithPlatform(ctx context.Context, ref string, platform *ocispec.Platform, onProgress func(PullProgress)) error {
	return c.pullWithMirrorFallback(ctx, ref, platform, onProgress)
}

func canonicalRegistryHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if idx := strings.IndexByte(h, '/'); idx >= 0 {
		h = h[:idx]
	}
	switch h {
	case "docker.io", "dockerhub", "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	default:
		return h
	}
}

// registryAuthForRef 根据镜像引用匹配全局配置中的仓库认证并编码为 X-Registry-Auth。
func registryAuthForRef(ref string) string {
	cfg := config.Get()
	if cfg == nil || len(cfg.Registry.Auth) == 0 {
		return ""
	}

	targetHost := ""
	if named, err := reference.ParseNormalizedNamed(ref); err == nil {
		targetHost = canonicalRegistryHost(reference.Domain(named))
	}
	if targetHost == "" {
		return ""
	}

	for _, auth := range cfg.Registry.Auth {
		if strings.TrimSpace(auth.Username) == "" && strings.TrimSpace(auth.Token) == "" {
			continue
		}
		if canonicalRegistryHost(auth.Host) != targetHost {
			continue
		}
		serverAddr := strings.TrimSpace(auth.Host)
		if targetHost == "docker.io" {
			serverAddr = "https://index.docker.io/v1/"
		}
		encoded, err := authconfig.Encode(registrytypes.AuthConfig{
			Username:      auth.Username,
			Password:      auth.Token,
			ServerAddress: serverAddr,
		})
		if err == nil {
			return encoded
		}
	}
	return ""
}

func isDigestedImageRef(ref string) bool {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return false
	}
	_, ok := named.(reference.Digested)
	return ok
}

// pullWithMirrorFallback 核心拉取逻辑：mirror 顺序尝试 + 官方 fallback + 自动 retag。
func (c *Client) pullWithMirrorFallback(ctx context.Context, ref string, platform *ocispec.Platform, onProgress func(PullProgress)) error {
	mirrors := EnabledMirrorHosts()
	canUseMirror := IsDockerHubImage(ref) && !isDigestedImageRef(ref) && len(mirrors) > 0

	if !canUseMirror {
		return c.pullSingleAttempt(ctx, ref, platform, onProgress)
	}

	var lastErr error
	for _, host := range mirrors {
		mirrorRef := RewriteRefToMirror(ref, host)
		if mirrorRef == ref {
			continue
		}
		logger.Logger.Info("尝试通过 mirror 拉取镜像",
			zap.String("originalRef", ref),
			zap.String("mirror", host),
			zap.String("mirrorRef", mirrorRef))

		if err := c.pullSingleAttempt(ctx, mirrorRef, platform, onProgress); err != nil {
			lastErr = err
			logger.Logger.Warn("mirror 拉取失败，尝试下一个",
				zap.String("mirror", host),
				zap.Error(err))
			continue
		}

		retagTarget := ref
		skipRetag := false
		if named, perr := reference.ParseNormalizedNamed(ref); perr == nil {
			if _, isDigested := named.(reference.Digested); isDigested {
				if tagged, isTagged := named.(reference.NamedTagged); isTagged {
					retagTarget = named.Name() + ":" + tagged.Tag()
				} else {
					// 纯 digest 引用 (repo@sha256:...) 无法作为 Docker ImageTag target，保留拉取到本地的 mirror 镜像
					skipRetag = true
				}
			}
		}

		if !skipRetag {
			if _, err := c.docker.ImageTag(ctx, client.ImageTagOptions{
				Source: mirrorRef,
				Target: retagTarget,
			}); err != nil {
				lastErr = fmt.Errorf("mirror %s retag 失败: %w", host, err)
				logger.Logger.Warn("mirror 拉取成功但 retag 失败，尝试下一个或回退",
					zap.String("mirrorRef", mirrorRef),
					zap.String("targetRef", retagTarget),
					zap.Error(err))
				_, _ = c.docker.ImageRemove(ctx, mirrorRef, client.ImageRemoveOptions{Force: false, PruneChildren: false})
				continue
			}

			_, _ = c.docker.ImageRemove(ctx, mirrorRef, client.ImageRemoveOptions{Force: false, PruneChildren: false})
		}
		c.InvalidateImageCache()
		logger.Logger.Info("通过 mirror 拉取镜像成功",
			zap.String("originalRef", ref),
			zap.String("mirror", host))
		return nil
	}

	logger.Logger.Warn("所有 mirror 拉取失败，回退到官方 registry",
		zap.String("ref", ref),
		zap.NamedError("lastMirrorError", lastErr))
	return c.pullSingleAttempt(ctx, ref, platform, onProgress)
}

// pullSingleAttempt 执行单次 pull，并解析流中的 errorDetail。
func (c *Client) pullSingleAttempt(ctx context.Context, ref string, platform *ocispec.Platform, onProgress func(PullProgress)) error {
	opts := client.ImagePullOptions{
		RegistryAuth: registryAuthForRef(ref),
	}
	if platform != nil && platform.OS != "" && platform.Architecture != "" {
		opts.Platforms = []ocispec.Platform{*platform}
	}
	rc, err := c.docker.ImagePull(ctx, ref, opts)
	if err != nil {
		return err
	}
	defer rc.Close()

	decoder := json.NewDecoder(rc)
	for {
		var progress PullProgress
		if derr := decoder.Decode(&progress); derr != nil {
			if derr == io.EOF {
				c.InvalidateImageCache()
				return nil
			}
			return derr
		}
		if progress.Error != "" || progress.ErrorDetail != nil {
			msg := progress.Error
			if progress.ErrorDetail != nil && progress.ErrorDetail.Message != "" {
				msg = progress.ErrorDetail.Message
			}
			return fmt.Errorf("pull %s 失败: %s", ref, msg)
		}
		if onProgress != nil {
			onProgress(progress)
		}
	}
}

// ImageInspect 检查镜像是否存在，如果存在返回镜像信息，不存在返回错误
func (c *Client) ImageInspect(ctx context.Context, ref string) (*image.InspectResponse, error) {
	res, err := c.docker.ImageInspect(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &res.InspectResponse, nil
}

func normalizeComparableRef(ref string) string {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return strings.TrimSpace(ref)
	}
	named = reference.TagNameOnly(named)
	return named.String()
}

// ImageExists 检查镜像引用是否存在（通过标准化匹配 RepoTags，正确处理带 :port 的仓库）
func (c *Client) ImageExists(ctx context.Context, imageRef string) (bool, error) {
	images, err := c.ListImages(ctx)
	if err != nil {
		return false, err
	}

	normalizedTarget := normalizeComparableRef(imageRef)

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageRef || normalizeComparableRef(tag) == normalizedTarget {
				return true, nil
			}
		}
	}

	return false, nil
}

// ListImages 列出本地镜像
func (c *Client) ListImages(ctx context.Context) ([]ImageInfo, error) {
	listRes, err := c.docker.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}
	res := make([]ImageInfo, 0, len(listRes.Items))
	for _, im := range listRes.Items {
		res = append(res, ImageInfo{
			ID:          im.ID,
			RepoTags:    im.RepoTags,
			RepoDigests: im.RepoDigests,
			Size:        im.Size,
			Created:     im.Created,
		})
	}
	return res, nil
}

// RemoveImage 删除镜像（未被使用时可删除）。ref 可为镜像ID或引用。
func (c *Client) RemoveImage(ctx context.Context, ref string, force bool, pruneChildren bool) error {
	_, err := c.docker.ImageRemove(ctx, ref, client.ImageRemoveOptions{Force: force, PruneChildren: pruneChildren})
	if err == nil {
		c.InvalidateImageCache()
	}
	return err
}

// ExportImage 导出镜像为 tar 包流
func (c *Client) ExportImage(ctx context.Context, ref string) (io.ReadCloser, error) {
	return c.docker.ImageSave(ctx, []string{ref})
}

type dockerStreamMessage struct {
	Error       string `json:"error"`
	ErrorDetail *struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
	Stream string `json:"stream"`
}

func decodeDockerJSONStreamError(r io.Reader, op string) error {
	dec := json.NewDecoder(r)
	for {
		var msg dockerStreamMessage
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("%s decode response: %w", op, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("%s failed: %s", op, msg.Error)
		}
		if msg.ErrorDetail != nil && msg.ErrorDetail.Message != "" {
			return fmt.Errorf("%s failed: %s", op, msg.ErrorDetail.Message)
		}
	}
}

func (c *Client) ImportImage(ctx context.Context, source io.Reader, repository string, tag string) error {
	var ref string
	if tag != "" {
		ref = repository + ":" + tag
	} else {
		ref = repository + ":latest"
	}

	importSource := client.ImageImportSource{
		Source:     source,
		SourceName: "-",
	}

	options := client.ImageImportOptions{
		Message: "Imported via watch-docker",
	}

	response, err := c.docker.ImageImport(ctx, importSource, ref, options)
	if err != nil {
		return err
	}
	defer response.Close()

	if err := decodeDockerJSONStreamError(response, "import image"); err != nil {
		return err
	}
	c.InvalidateImageCache()
	return nil
}

// LoadImage 从 tar 包流加载镜像
func (c *Client) LoadImage(ctx context.Context, source io.Reader) error {
	response, err := c.docker.ImageLoad(ctx, source, client.ImageLoadWithQuiet(true))
	if err != nil {
		return err
	}
	defer response.Close()

	if err := decodeDockerJSONStreamError(response, "load image"); err != nil {
		return err
	}
	c.InvalidateImageCache()
	return nil
}
