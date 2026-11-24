package scanner

import (
	"context"
	"strings"
	"sync"
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
	ImageID       string                    `json:"imageID,omitempty"`
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

	snapshotMu sync.RWMutex
	lastScanAt time.Time
}

func New(d *dockercli.Client, r *registry.Client) *Scanner {
	return &Scanner{docker: d, registry: r}
}

// ScanOnce 扫描当前主机上的容器，返回其更新状态。
func (s *Scanner) ScanOnce(ctx context.Context, includeStopped bool, concurrency int, isUserCache bool, isHaveUpdate bool) ([]ContainerStatus, error) {
	containers, err := s.docker.ListContainers(ctx, includeStopped)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]ContainerStatus, len(containers))
	cfg := config.Get()

	type containerEval struct {
		containerIdx  int
		query         registry.DigestQuery
		repoDigests   []string
		imageID       string
		skippedUpdate bool
	}

	queryMap := make(map[string]registry.DigestQuery)
	evalsByQuery := make(map[string][]containerEval)

	for i, ct := range containers {
		st := ContainerStatus{
			ID:            ct.ID,
			Name:          ct.Name,
			Image:         ct.Image,
			ImageID:       ct.ImageID,
			Running:       strings.EqualFold(ct.State, "running"),
			Labels:        ct.Labels,
			LastCheckedAt: now,
			StartedAt:     ct.StartedAt,
			Ports:         ct.Ports,
		}

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

		q := registry.DigestQuery{
			ImageRef:         ct.Image,
			OS:               ct.ImageOS,
			Arch:             ct.ImageArch,
			Variant:          ct.ImageVariant,
			NeedConfigDigest: len(ct.RepoDigests) == 0 && !ct.IsLocalImage,
		}
		qKey := q.Key()
		if existing, ok := queryMap[qKey]; ok {
			if q.NeedConfigDigest && !existing.NeedConfigDigest {
				existing.NeedConfigDigest = true
				queryMap[qKey] = existing
			}
		} else {
			queryMap[qKey] = q
		}

		evalsByQuery[qKey] = append(evalsByQuery[qKey], containerEval{
			containerIdx:  i,
			query:         q,
			repoDigests:   ct.RepoDigests,
			imageID:       ct.ImageID,
			skippedUpdate: dec.SkippedUpdate,
		})
	}

	if len(queryMap) == 0 {
		return result, nil
	}

	queries := make([]registry.DigestQuery, 0, len(queryMap))
	for _, q := range queryMap {
		queries = append(queries, q)
	}

	cacheOnly := !isHaveUpdate
	if !isUserCache && !cacheOnly {
		logger.Logger.Debug("批量扫描镜像",
			zap.Int("totalContainers", len(containers)),
			zap.Int("uniqueQueries", len(queries)))
	}

	digestResults := s.registry.GetRemoteDigestsBatchWithPlatforms(ctx, queries, isUserCache || cacheOnly, cacheOnly, concurrency)

	for qKey, evals := range evalsByQuery {
		digestResult := digestResults[qKey]

		for _, info := range evals {
			ct := containers[info.containerIdx]
			st := ContainerStatus{
				ID:            ct.ID,
				Name:          ct.Name,
				Image:         ct.Image,
				ImageID:       ct.ImageID,
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
					logger.Logger.Warn("get remote digest: rate limited", zap.String("image", ct.Image))
				case registry.ErrorTypeNotFound:
					st.Status = "Error"
					st.SkipReason = "镜像或标签不存在"
					logger.Logger.Warn("get remote digest: not found", zap.String("image", ct.Image))
				default:
					st.Status = "Error"
					st.SkipReason = "registry: " + digestResult.Error.Error()
					logger.Logger.Error("get remote digest", zap.String("image", ct.Image), logger.ZapErr(digestResult.Error))
				}
				result[info.containerIdx] = st
				continue
			}

			chosen := digestResult.IndexDigest
			if chosen == "" {
				chosen = digestResult.ChildDigest
			}
			st.RemoteDigest = chosen

			// 当 cacheOnly 且尚未有远端 digest 缓存时，视为当前已知（不误报 UpdateAvailable）
			if chosen == "" && digestResult.ConfigDigest == "" {
				st.Status = "UpToDate"
				result[info.containerIdx] = st
				continue
			}

			upToDate := false
			if len(st.CurrentDigest) > 0 {
				// 同时兼容本地存储 manifest list digest (IndexDigest) 或单平台 child manifest digest (ChildDigest)
				upToDate = compareDigests(st.CurrentDigest, digestResult.IndexDigest) ||
					compareDigests(st.CurrentDigest, digestResult.ChildDigest)
			} else if digestResult.ConfigDigest != "" && info.imageID != "" {
				// Mirror 拉取 retag+untag 后 RepoDigests 可能为空，此时比对镜像 Config Digest (ImageID) 消除无限更新循环
				upToDate = strings.EqualFold(info.imageID, digestResult.ConfigDigest)
				if upToDate {
					st.CurrentDigest = []string{info.imageID}
				}
			}

			if upToDate {
				st.Status = "UpToDate"
			} else {
				st.Status = "UpdateAvailable"
			}
			result[info.containerIdx] = st
		}
	}

	if !cacheOnly {
		s.snapshotMu.Lock()
		s.lastScanAt = now
		s.snapshotMu.Unlock()
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

	if strings.HasPrefix(v, "sha256:") {
		return v
	}

	return ""
}

// GetRegistryClient 返回 registry 客户端（用于动态更新凭据）
func (s *Scanner) GetRegistryClient() *registry.Client {
	return s.registry
}
