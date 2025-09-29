package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// setupImageRoutes 设置镜像相关的路由
func (s *Server) setupImageRoutes(protected *gin.RouterGroup) {
	protected.GET("/images", s.handleListImages())
	protected.DELETE("/images", s.handleDeleteImage())
	protected.GET("/images/:id/download", s.handleDownloadImage())
	protected.POST("/images/import", s.handleImportImage())
}

func (s *Server) handleListImages() gin.HandlerFunc {
	return func(c *gin.Context) {
		imgs, err := s.docker.ListImages(c.Request.Context())
		if err != nil {
			s.logger.Error("list images", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"images": imgs}))
	}
}

// handleDeleteImage 删除未使用的镜像（需前端确认未被使用）
// body: { "ref": "imageID or repo:tag", "force": false }
func (s *Server) handleDeleteImage() gin.HandlerFunc {
	type req struct {
		Ref   string `json:"ref"`
		Force bool   `json:"force"`
	}
	return func(c *gin.Context) {
		var r req
		if err := c.ShouldBindJSON(&r); err != nil || r.Ref == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "ref required"))
			return
		}
		if err := s.docker.RemoveImage(c.Request.Context(), r.Ref, r.Force, true); err != nil {
			s.logger.Error("remove image", zap.String("ref", r.Ref), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handleDownloadImage 处理镜像下载
func (s *Server) handleDownloadImage() gin.HandlerFunc {
	return func(c *gin.Context) {
		imageID := c.Param("id")
		if imageID == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "image id required"))
			return
		}

		ctx := c.Request.Context()

		// 获取镜像信息以生成文件名
		images, err := s.docker.ListImages(ctx)
		if err != nil {
			s.logger.Error("list images for download", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取镜像信息失败"))
			return
		}

		var targetImage *dockercli.ImageInfo
		var imageRef string

		// 先尝试完全匹配ID
		for _, img := range images {
			if img.ID == imageID {
				targetImage = &img
				imageRef = img.ID
				break
			}
		}

		// 如果没找到，尝试短ID匹配
		if targetImage == nil {
			for _, img := range images {
				if strings.HasSuffix(img.ID, imageID) {
					targetImage = &img
					imageRef = img.ID
					break
				}
			}
		}

		// 如果还没找到，尝试通过tag匹配
		if targetImage == nil {
			for _, img := range images {
				for _, tag := range img.RepoTags {
					if tag != "<none>:<none>" && strings.Contains(tag, imageID) {
						targetImage = &img
						imageRef = tag // 使用tag作为引用
						break
					}
				}
				if targetImage != nil {
					break
				}
			}
		}

		if targetImage == nil {
			s.logger.Error("image not found", zap.String("requestedID", imageID), zap.Int("totalImages", len(images)))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("镜像不存在: %s", imageID)))
			return
		}

		s.logger.Info("found target image", zap.String("requestedID", imageID), zap.String("actualID", targetImage.ID), zap.String("imageRef", imageRef), zap.Any("repoTags", targetImage.RepoTags))

		// 导出镜像，优先使用有效的tag，然后使用ID
		exportRef := imageRef
		if len(targetImage.RepoTags) > 0 {
			for _, tag := range targetImage.RepoTags {
				if tag != "<none>:<none>" {
					exportRef = tag
					break
				}
			}
		}

		s.logger.Info("exporting image", zap.String("exportRef", exportRef))
		reader, err := s.docker.ExportImage(ctx, exportRef)
		if err != nil {
			s.logger.Error("export image failed", zap.String("exportRef", exportRef), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "导出镜像失败: "+err.Error()))
			return
		}
		defer reader.Close()

		// 生成文件名
		filename := generateImageFileName(targetImage)

		s.logger.Info("starting image download", zap.String("imageID", imageID), zap.String("filename", filename))

		// 设置响应头
		c.Header("Content-Type", "application/x-tar")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Header("Content-Transfer-Encoding", "binary")

		// 流式传输文件
		_, err = io.Copy(c.Writer, reader)
		if err != nil {
			s.logger.Error("copy image tar stream", zap.String("imageID", imageID), zap.Error(err))
			return
		}

		s.logger.Info("image download completed", zap.String("imageID", imageID))
	}
}

// generateImageFileName 根据镜像信息生成下载文件名
func generateImageFileName(image *dockercli.ImageInfo) string {
	// 优先使用 repoTag
	if len(image.RepoTags) > 0 {
		for _, tag := range image.RepoTags {
			if tag != "<none>:<none>" {
				// 替换不合法的文件名字符
				filename := strings.ReplaceAll(tag, ":", "_")
				filename = strings.ReplaceAll(filename, "/", "_")
				return fmt.Sprintf("%s.tar", filename)
			}
		}
	}

	// 如果没有有效标签，使用短ID
	shortID := image.ID
	if strings.HasPrefix(shortID, "sha256:") {
		shortID = shortID[7:19]
	} else if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	return fmt.Sprintf("image_%s.tar", shortID)
}

// handleImportImage 处理镜像导入
func (s *Server) handleImportImage() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 解析multipart/form-data
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			s.logger.Error("get upload file", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "获取上传文件失败"))
			return
		}
		defer file.Close()

		// 验证文件类型（可选）
		if header.Size == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "文件为空"))
			return
		}

		s.logger.Info("starting image import",
			zap.String("filename", header.Filename),
			zap.Int64("size", header.Size))

		ctx := c.Request.Context()

		// 导入镜像
		err = s.docker.LoadImage(ctx, file)
		if err != nil {
			s.logger.Error("import image failed",
				zap.String("filename", header.Filename),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "导入镜像失败: "+err.Error()))
			return
		}

		s.logger.Info("image import completed", zap.String("filename", header.Filename))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"message": "镜像导入成功"}))
	}
}
