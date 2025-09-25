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
)

type Decision struct {
	// Skipped 表示此次应跳过更新检查及后续更新动作
	Skipped bool
	// Reason 给出跳过的原因，便于对外呈现（例如 "pinned digest"）
	Reason string
	// Force 为 true 时表示外部标记了强制更新（如 label），即便命中某些跳过条件也应继续
	Force bool
}

type Input struct {
	// ImageRef 容器镜像引用，如 repo:tag 或 repo@sha256:xxx
	ImageRef string
	// RepoDigests 本地镜像的 RepoDigests（为空时常代表本地构建未推送）
	RepoDigests []string
	// Labels 容器标签，支持策略控制（watchdocker.skip/watchdocker.force 等）
	Labels map[string]string
	// FloatingTags 配置的“浮动标签”列表；若非空，仅这些 tag 会被纳入更新检查
	FloatingTags []string
	// SkipLocal 开启时对“本地构建镜像”（无 RepoDigests）跳过
	SkipLocal bool
	// SkipPinned 开启时对通过 digest 固定的镜像（image@sha256:...）跳过
	SkipPinned bool
	// SkipSemver 开启时对严格语义化版本标签（1.2.3 / v1.2.3）跳过
	SkipSemver bool
	// OnlyLabels 仅当容器包含这些 label（或精确键值）之一时才纳入检查
	OnlyLabels []string
	// SkipLabels 容器包含这些 label（或精确键值）则跳过
	SkipLabels []string
	// AllowComposeUpdate 允许对 Compose 管理的容器进行更新
	AllowComposeUpdate bool
}

var semverStrict = regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)$`)

// Evaluate 按输入条件与策略计算是否跳过。
// 判定顺序遵循“显式 > 过滤 > 来源/固定 > 版本/标签”的原则，以便快速短路。
func Evaluate(in Input) Decision {
	// 1) 显式控制：label 强制跳过或强制更新
	if val := strings.ToLower(in.Labels["watchdocker.skip"]); val == "true" {
		return Decision{Skipped: true, Reason: "label skip"}
	}
	// force update even if pinned
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

	// 3) 编排器：Compose 管理的容器默认跳过（避免与外部编排冲突）
	if in.Labels["com.docker.compose.project"] != "" && !in.AllowComposeUpdate {
		return Decision{Skipped: true, Reason: "compose managed"}
	}

	// 4) 来源/固定：digest 固定、本地构建
	if in.SkipPinned && strings.Contains(in.ImageRef, "@sha256:") {
		return Decision{Skipped: true, Reason: "pinned digest"}
	}
	// skip local build (no RepoDigests)
	if in.SkipLocal && len(in.RepoDigests) == 0 {
		return Decision{Skipped: true, Reason: "local build"}
	}

	// 5) 标签与版本：严格语义化版本、浮动标签白名单
	if in.SkipSemver && isStrictSemverTag(in.ImageRef) {
		return Decision{Skipped: true, Reason: "pinned semver"}
	}

	// 若配置了 FloatingTags，则仅这些 tag 会被检查；不在列表中则跳过
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

// isStrictSemverTag 判断镜像标签是否为严格语义化版本（1.2.3 / v1.2.3）。
func isStrictSemverTag(ref string) bool {
	tag := imageTag(ref)
	if !semverStrict.MatchString(tag) {
		return false
	}
	_, err := semver.Parse(strings.TrimPrefix(tag, "v"))
	return err == nil
}

// imageTag 从镜像引用中提取 tag，未显式给出时返回 "latest"。
func imageTag(ref string) string {
	at := strings.LastIndex(ref, ":")
	if at == -1 {
		return "latest"
	}
	return ref[at+1:]
}
