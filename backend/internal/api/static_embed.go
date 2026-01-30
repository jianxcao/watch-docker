// +build !docker

package api

import "embed"

// staticFS 嵌入前端静态资源
// 仅在非 Docker 构建时使用
//
//go:embed static
var staticFS embed.FS
