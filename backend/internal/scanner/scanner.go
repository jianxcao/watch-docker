package scanner

import (
	"context"
	"strings"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/policy"
	"github.com/jianxcao/watch-docker/backend/internal/registry"
	"go.uber.org/zap"
)

type ContainerStatus struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Image         string                    `json:"image"`
	Running       bool                      `json:"running"`
	CurrentDigest []string                  `json:"currentDigest"`
	RemoteDigest  string                    `json:"remoteDigest"`
	Status        string                    `json:"status"` // UpToDate | UpdateAvailable | Skipped | Error
	Skipped       bool                      `json:"skipped"`
	SkippedUpdate bool                      `json:"skippedUpdate"`
	SkipReason    string                    `json:"skipReason"`
	ErrorType     string                    `json:"errorType,omitempty"` // not_found | rate_limited | general
	Labels        map[string]string         `json:"labels"`
	LastCheckedAt time.Time                 `json:"lastCheckedAt"`
	StartedAt     string                    `json:"startedAt"`
	Ports         []dockercli.PortInfo      `json:"ports"`
	Stats         *dockercli.ContainerStats `json:"stats,omitempty"`
}

type Scanner struct {
	docker   *dockercli.Client
	registry *registry.Client
}

func New(d *dockercli.Client, r *registry.Client) *Scanner {
	return &Scanner{docker: d, registry: r}
}

// ScanOnce 扫描当前主机上的容器，返回其更新状态。
//
// 关键流程：
// 1) 通过 Docker 客户端获取容器列表
// 2) 对每个容器先做策略评估（policy.Evaluate），尽早跳过无需查询 registry 的容器
// 3) 批量获取所有需要检查的镜像的远端 digest（使用批量模式）
// 4) 与本地 RepoDigest 对比，生成 UpToDate/UpdateAvailable/Skipped/Error 状态
func (s *Scanner) ScanOnce(ctx context.Context, includeStopped bool, concurrency int, isUserCache bool, isHaveUpdate bool) ([]ContainerStatus, error) {
	containers, err := s.docker.ListContainers(ctx, includeStopped)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]ContainerStatus, len(containers))
	cfg := config.Get()

	// 1. 策略评估 + 收集需要查询的镜像
	type containerInfo struct {
		containerIdx  int
		image         string
		repoDigests   []string
		skippedUpdate bool
	}
	imageToContainers := make(map[string][]containerInfo)

	for i, ct := range containers {
		st := ContainerStatus{
			ID:            ct.ID,
			Name:          ct.Name,
			Image:         ct.Image,
			Running:       strings.EqualFold(ct.State, "running"),
			Labels:        ct.Labels,
			LastCheckedAt: now,
			StartedAt:     ct.StartedAt,
			Ports:         ct.Ports,
		}

		// policy evaluation
		dec := policy.Evaluate(policy.Input{
			ImageRef:           ct.Image,
			RepoDigests:        ct.RepoDigests,
			IsLocalImage:       ct.IsLocalImage,
			Labels:             ct.Labels,
			FloatingTags:       cfg.Policy.FloatingTags,
			SkipLocal:          cfg.Policy.SkipLocalBuild,
			SkipPinned:         cfg.Policy.SkipPinnedDigest,
			SkipSemver:         cfg.Policy.SkipSemverPinned,
			OnlyLabels:         cfg.Policy.OnlyLabels,
			SkipLabels:         cfg.Policy.SkipLabels,
			AllowComposeUpdate: cfg.Scan.AllowComposeUpdate,
		})

		if dec.Skipped && !dec.Force {
			st.Skipped = true
			st.SkipReason = dec.Reason
			st.Status = "Skipped"
			st.CurrentDigest = ct.RepoDigests
			result[i] = st
			continue
		}

		// 需要查询的容器。是否允许远程查询由 isHaveUpdate 控制。
		imageToContainers[ct.Image] = append(imageToContainers[ct.Image], containerInfo{
			containerIdx:  i,
			image:         ct.Image,
			repoDigests:   ct.RepoDigests,
			skippedUpdate: dec.SkippedUpdate,
		})
	}

	// 2. 批量查询所有镜像
	if len(imageToContainers) == 0 {
		return result, nil
	}

	imagesToQuery := make([]string, 0, len(imageToContainers))
	for img := range imageToContainers {
		imagesToQuery = append(imagesToQuery, img)
	}

	logger.Logger.Debug("批量扫描镜像",
		zap.Int("totalContainers", len(containers)),
		zap.Int("uniqueImages", len(imagesToQuery)))

	cacheOnly := !isHaveUpdate
	digestResults := s.registry.GetRemoteDigestsBatch(ctx, imagesToQuery, isUserCache || cacheOnly, cacheOnly, concurrency)

	// 3. 填充结果
	for image, infos := range imageToContainers {
		digestResult := digestResults[image]

		for _, info := range infos {
			ct := containers[info.containerIdx]
			st := ContainerStatus{
				ID:            ct.ID,
				Name:          ct.Name,
				Image:         ct.Image,
				Running:       strings.EqualFold(ct.State, "running"),
				Labels:        ct.Labels,
				LastCheckedAt: now,
				StartedAt:     ct.StartedAt,
				Ports:         ct.Ports,
				CurrentDigest: info.repoDigests,
				SkippedUpdate: info.skippedUpdate,
			}

			if digestResult.Error != nil {
				errType := string(digestResult.ErrType)
				if errType == "" {
					errType = string(registry.ErrorTypeGeneral)
				}
				st.ErrorType = errType

				switch digestResult.ErrType {
				case registry.ErrorTypeRateLimited:
					st.Status = "Error"
					st.SkipReason = "registry 请求频率超限，请稍后重试"
					logger.Logger.Warn("get remote digest: rate limited",
						zap.String("image", image))
				case registry.ErrorTypeNotFound:
					st.Status = "Error"
					st.SkipReason = "镜像或标签不存在"
					logger.Logger.Warn("get remote digest: not found",
						zap.String("image", image))
				default:
					st.Status = "Error"
					st.SkipReason = "registry: " + digestResult.Error.Error()
					logger.Logger.Error("get remote digest",
						zap.String("image", image),
						logger.ZapErr(digestResult.Error))
				}
				result[info.containerIdx] = st
				continue
			}

			chosen := digestResult.IndexDigest
			if chosen == "" {
				chosen = digestResult.ChildDigest
			}
			st.RemoteDigest = chosen

			if len(st.CurrentDigest) == 0 || (st.RemoteDigest != "" && !compareDigests(st.CurrentDigest, st.RemoteDigest)) {
				st.Status = "UpdateAvailable"
			} else {
				st.Status = "UpToDate"
			}
			result[info.containerIdx] = st
		}
	}

	return result, nil
}

func compareDigests(currentDigests []string, remoteDigest string) bool {
	if remoteDigest == "" {
		return false
	}

	for _, d := range currentDigests {
		localDigest := extractDigest(d)
		if localDigest != "" && localDigest == remoteDigest {
			return true
		}
	}
	return false
}

func extractDigest(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}

	if strings.Contains(v, "@") {
		parts := strings.SplitN(v, "@", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	// 支持仅有 digest 的格式，例如 "sha256:...."
	if strings.HasPrefix(v, "sha256:") {
		return v
	}

	return ""
}

// GetRegistryClient 返回 registry 客户端（用于动态更新凭据）
func (s *Scanner) GetRegistryClient() *registry.Client {
	return s.registry
}
