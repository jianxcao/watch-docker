// +build docker

package api

import "embed"

// staticFS 为 Docker 构建提供空的 embed.FS
// Docker 环境使用 /app/static 目录，不需要嵌入资源
var staticFS embed.FS
