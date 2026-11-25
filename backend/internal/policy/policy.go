// Package policy 实现容器更新的策略判定：
// - 依据容器 label、镜像来源（本地构建/固定 digest）、标签形态（严格语义化版本/浮动标签）
// - 可选的 only/exclude label 过滤
// - 对 Compose 管理的容器默认跳过（可配置允许）
// 通过 Evaluate 返回跳过与否、原因以及是否强制更新标记。
package policy

import (
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/distribution/reference"
)

type Decision struct {
	Skipped       bool
	Reason        string
	Force         bool
	SkippedUpdate bool
}

type Input struct {
	ImageRef           string
	RepoDigests        []string
	IsLocalImage       bool
	Labels             map[string]string
	FloatingTags       []string
	SkipLocal          bool
	SkipPinned         bool
	SkipSemver         bool
	OnlyLabels         []string
	SkipLabels         []string
	AllowComposeUpdate bool
}

var semverStrict = regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)$`)

// Evaluate 按输入条件与策略计算是否跳过。
func Evaluate(in Input) Decision {
	// 1) 显式控制：label 强制跳过或强制更新
	if val := strings.ToLower(in.Labels["watchdocker.skip"]); val == "true" {
		return Decision{Skipped: true, Reason: "label skip"}
	}
	if val := strings.ToLower(in.Labels["watchdocker.skipUpdate"]); val == "true" {
		return Decision{SkippedUpdate: true, Reason: "label skip update"}
	}
	if val := strings.ToLower(in.Labels["watchdocker.force"]); val == "true" {
		return Decision{Skipped: false, Force: true}
	}

	// 2) 过滤：only / exclude label
	if len(in.OnlyLabels) > 0 {
		matched := false
		for _, kv := range in.OnlyLabels {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				if v, ok := in.Labels[parts[0]]; ok && v == parts[1] {
					matched = true
					break
				}
			} else {
				if _, ok := in.Labels[kv]; ok {
					matched = true
					break
				}
			}
		}
		if !matched {
			return Decision{Skipped: true, Reason: "onlyLabels filter"}
		}
	}

	for _, kv := range in.SkipLabels {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			if v, ok := in.Labels[parts[0]]; ok && v == parts[1] {
				return Decision{Skipped: true, Reason: "label skip"}
			}
		} else {
			if _, ok := in.Labels[kv]; ok {
				return Decision{Skipped: true, Reason: "label skip"}
			}
		}
	}

	// 3) 编排器：Compose 管理的容器默认跳过
	if in.Labels["com.docker.compose.project"] != "" && !in.AllowComposeUpdate {
		return Decision{Skipped: true, Reason: "compose managed"}
	}

	// 4) 来源/固定：digest 固定、本地构建
	if in.SkipPinned && isPinnedDigest(in.ImageRef) {
		return Decision{Skipped: true, Reason: "pinned digest"}
	}
	if in.SkipLocal && in.IsLocalImage {
		return Decision{Skipped: true, Reason: "local build"}
	}

	// 5) 标签与版本：严格语义化版本、浮动标签白名单
	if in.SkipSemver && isStrictSemverTag(in.ImageRef) {
		return Decision{Skipped: true, Reason: "pinned semver"}
	}

	if len(in.FloatingTags) > 0 {
		tag := imageTag(in.ImageRef)
		found := false
		for _, t := range in.FloatingTags {
			if strings.EqualFold(t, tag) {
				found = true
				break
			}
		}
		if !found {
			return Decision{Skipped: true, Reason: "tag not in floating list"}
		}
	}
	return Decision{}
}

func isPinnedDigest(ref string) bool {
	if strings.Contains(ref, "@sha256:") {
		return true
	}
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return false
	}
	_, ok := named.(reference.Digested)
	return ok
}

// isStrictSemverTag 判断镜像标签是否为严格语义化版本（1.2.3 / v1.2.3）。
func isStrictSemverTag(ref string) bool {
	tag := imageTag(ref)
	if tag == "" || !semverStrict.MatchString(tag) {
		return false
	}
	_, err := semver.Parse(strings.TrimPrefix(tag, "v"))
	return err == nil
}

// imageTag 使用 distribution/reference 安全提取 tag，正确兼容 host:port/org/app 等带端口引用。
func imageTag(ref string) string {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		// 解析失败时的兜底：仅在最后一个 '/' 之后寻找 ':'
		lastSlash := strings.LastIndex(ref, "/")
		tail := ref
		if lastSlash >= 0 {
			tail = ref[lastSlash+1:]
		}
		if idx := strings.LastIndex(tail, ":"); idx >= 0 {
			return tail[idx+1:]
		}
		return "latest"
	}
	if tagged, ok := named.(reference.Tagged); ok {
		return tagged.Tag()
	}
	if _, ok := named.(reference.Digested); ok {
		return ""
	}
	return "latest"
}
