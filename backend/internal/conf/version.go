package conf

import "fmt"

// 版本信息 - 通过 ldflags 在构建时注入
// 使用方式: go build -ldflags="-X github.com/jianxcao/watch-docker/backend/internal/conf.Version=v1.0.0 -X github.com/jianxcao/watch-docker/backend/internal/conf.Commit=$(git rev-parse HEAD) -X github.com/jianxcao/watch-docker/backend/internal/conf.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"

var (
	// Version 版本号，默认 dev，仅在构建时通过 ldflags 注入
	Version = "dev"

	// Commit Git 提交 SHA，默认 unknown
	Commit = "unknown"

	// BuildTime 构建时间，默认 unknown
	BuildTime = "unknown"
)

// GetVersion 返回格式化的版本信息
func GetVersion() string {
	if Commit != "unknown" && BuildTime != "unknown" {
		return fmt.Sprintf("%s (commit: %s, built at: %s)", Version, Commit, BuildTime)
	}
	return Version
}
