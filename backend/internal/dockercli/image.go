package dockercli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
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
	Status         string          `json:"status"`
	ID             string          `json:"id"`
	Progress       string          `json:"progress"`
	ProgressDetail *ProgressDetail `json:"progressDetail,omitempty"`
	// 当 daemon 拉取出错时，下面两个字段会被填充（pull 流式响应里的错误事件）
	Error       string             `json:"error,omitempty"`
	ErrorDetail *PullErrorDetail   `json:"errorDetail,omitempty"`
}

type ProgressDetail struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

type PullErrorDetail struct {
	Message string `json:"message"`
}

// ImagePull 拉取镜像（丢弃输出以避免阻塞）。
// 如果配置了 Docker Hub mirror 且 ref 是 docker.io 镜像，会按顺序尝试 mirror，
// 全部失败后回退到原始 ref。成功通过 mirror 拉取后会自动 retag 为原始 ref，
// 这样上层代码、容器配置都感知不到 mirror 的存在。
func (c *Client) ImagePull(ctx context.Context, ref string) error {
	return c.pullWithMirrorFallback(ctx, ref, nil)
}

// ImagePullWithProgress 拉取镜像并通过回调报告进度。详见 ImagePull 的说明。
func (c *Client) ImagePullWithProgress(ctx context.Context, ref string, onProgress func(PullProgress)) error {
	return c.pullWithMirrorFallback(ctx, ref, onProgress)
}

// pullWithMirrorFallback 核心拉取逻辑：mirror 顺序尝试 + 官方 fallback + 自动 retag。
func (c *Client) pullWithMirrorFallback(ctx context.Context, ref string, onProgress func(PullProgress)) error {
	mirrors := EnabledMirrorHosts()
	canUseMirror := IsDockerHubImage(ref) && len(mirrors) > 0

	// 非 docker.io 镜像或未配置 mirror，直接走原流程
	if !canUseMirror {
		return c.pullSingleAttempt(ctx, ref, onProgress)
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

		if err := c.pullSingleAttempt(ctx, mirrorRef, onProgress); err != nil {
			lastErr = err
			logger.Logger.Warn("mirror 拉取失败，尝试下一个",
				zap.String("mirror", host),
				zap.Error(err))
			continue
		}

		// 成功后立刻打上原始 tag，再尝试清理 mirror 临时 tag
		if err := c.docker.ImageTag(ctx, mirrorRef, ref); err != nil {
			logger.Logger.Warn("mirror 拉取成功但 retag 失败，仍按成功处理",
				zap.String("mirrorRef", mirrorRef),
				zap.String("targetRef", ref),
				zap.Error(err))
		} else {
			// 仅当 retag 成功时才 untag mirror 临时引用
			_, _ = c.docker.ImageRemove(ctx, mirrorRef, image.RemoveOptions{Force: false, PruneChildren: false})
		}
		logger.Logger.Info("通过 mirror 拉取镜像成功",
			zap.String("originalRef", ref),
			zap.String("mirror", host))
		return nil
	}

	logger.Logger.Warn("所有 mirror 拉取失败，回退到官方 registry",
		zap.String("ref", ref),
		zap.NamedError("lastMirrorError", lastErr))
	return c.pullSingleAttempt(ctx, ref, onProgress)
}

// pullSingleAttempt 执行单次 pull，并解析流中的 errorDetail。
func (c *Client) pullSingleAttempt(ctx context.Context, ref string, onProgress func(PullProgress)) error {
	rc, err := c.docker.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()

	decoder := json.NewDecoder(rc)
	for {
		var progress PullProgress
		if derr := decoder.Decode(&progress); derr != nil {
			if derr == io.EOF {
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
	img, err := c.docker.ImageInspect(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// ImageExists 检查镜像引用是否存在（通过匹配 RepoTags）
func (c *Client) ImageExists(ctx context.Context, imageRef string) (bool, error) {
	images, err := c.ListImages(ctx)
	if err != nil {
		return false, err
	}

	// 规范化镜像引用（如果没有 tag，默认添加 :latest）
	normalizedRef := imageRef
	if !strings.Contains(imageRef, ":") {
		normalizedRef = imageRef + ":latest"
	}

	// 检查镜像引用是否在 RepoTags 中
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageRef || tag == normalizedRef {
				return true, nil
			}
		}
	}

	return false, nil
}

// ListImages 列出本地镜像
func (c *Client) ListImages(ctx context.Context) ([]ImageInfo, error) {
	imgs, err := c.docker.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	res := make([]ImageInfo, 0, len(imgs))
	for _, im := range imgs {
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
	_, err := c.docker.ImageRemove(ctx, ref, image.RemoveOptions{Force: force, PruneChildren: pruneChildren})
	return err
}

// ExportImage 导出镜像为 tar 包流
func (c *Client) ExportImage(ctx context.Context, ref string) (io.ReadCloser, error) {
	return c.docker.ImageSave(ctx, []string{ref})
}

type ImportImageResponse struct {
	ErrorDetail *map[string]interface{} `json:"errorDetail"`
	Stream      *string                 `json:"stream"`
}

func (c *Client) ImportImage(ctx context.Context, source io.Reader, repository string, tag string) error {
	// 构建完整的镜像引用
	var ref string
	if tag != "" {
		ref = repository + ":" + tag
	} else {
		ref = repository + ":latest"
	}

	importSource := image.ImportSource{
		Source:     source,
		SourceName: "-", // 表示从标准输入读取
	}

	options := image.ImportOptions{
		Message: "Imported via watch-docker",
	}

	response, err := c.docker.ImageImport(ctx, importSource, ref, options)
	if err != nil {
		return err
	}
	defer response.Close()

	// 读取流转换成字符串显示
	body, err := io.ReadAll(response)
	if err != nil {
		return err
	}
	var importImageResponse ImportImageResponse
	err = json.Unmarshal(body, &importImageResponse)
	if err != nil {
		return err
	}
	if importImageResponse.ErrorDetail != nil {
		errorDetail := *importImageResponse.ErrorDetail
		msg := errorDetail["message"].(string)
		return fmt.Errorf("import image failed: %s", msg)
	}
	return nil
}

// ImportImage 从 tar 包流导入镜像
func (c *Client) LoadImage(ctx context.Context, source io.Reader) error {
	response, err := c.docker.ImageLoad(ctx, source, client.ImageLoadOption(client.ImageLoadWithQuiet(true)))
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// 读取流转换成字符串显示
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	var importImageResponse ImportImageResponse
	err = json.Unmarshal(body, &importImageResponse)
	if err != nil {
		return err
	}
	if importImageResponse.ErrorDetail != nil {
		errorDetail := *importImageResponse.ErrorDetail
		msg := errorDetail["message"].(string)
		return fmt.Errorf("import image failed: %s", msg)
	}
	return err
}
