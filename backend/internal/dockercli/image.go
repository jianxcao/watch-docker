package dockercli

import (
	"context"
	"io"

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

// ImportImage 从 tar 包流导入镜像
func (c *Client) ImportImage(ctx context.Context, source io.Reader) error {
	response, err := c.docker.ImageLoad(ctx, source, client.ImageLoadOption(client.ImageLoadWithQuiet(true)))
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// 读取响应以确保完成加载
	_, err = io.Copy(io.Discard, response.Body)
	return err
}
