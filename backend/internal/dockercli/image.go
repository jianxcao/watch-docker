package dockercli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// ImageInfo 镜像基础信息（用于列表展示）
type ImageInfo struct {
	ID          string   `json:"id"`
	RepoTags    []string `json:"repoTags"`
	RepoDigests []string `json:"repoDigests"`
	Size        int64    `json:"size"`
	Created     int64    `json:"created"`
}

// ImagePull 拉取镜像（丢弃输出以避免阻塞）
func (c *Client) ImagePull(ctx context.Context, ref string) error {
	rc, err := c.docker.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)
	return nil
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
