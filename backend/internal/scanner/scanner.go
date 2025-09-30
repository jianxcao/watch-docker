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
	SkipReason    string                    `json:"skipReason"`
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
// 3) 对需要检查的容器，并发获取远端 digest（受 concurrency 限制）
// 4) 与本地 RepoDigest 对比，生成 UpToDate/UpdateAvailable/Skipped/Error 状态
func (s *Scanner) ScanOnce(ctx context.Context, includeStopped bool, concurrency int, isUserCache bool, isHaveUpdate bool) ([]ContainerStatus, error) {
	containers, err := s.docker.ListContainers(ctx, includeStopped)
	if err != nil {
		return nil, err
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 64 {
		concurrency = 64
	}
	now := time.Now()
	result := make([]ContainerStatus, len(containers))

	type job struct{ idx int }
	jobs := make(chan job)
	done := make(chan struct{})

	// worker 按序从 jobs 中取任务。每个任务对应一个容器：
	// - 优先进行策略评估（避免不必要的远端请求）
	// - 需要检查时访问 registry 获取最新 digest
	// - 写入对应下标的结果，保持与输入容器顺序一致
	worker := func() {
		for j := range jobs {
			ct := containers[j.idx]
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
			cfg := config.Get()
			dec := policy.Evaluate(policy.Input{
				ImageRef:           ct.Image,
				RepoDigests:        ct.RepoDigests,
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
				result[j.idx] = st
				continue
			}
			st.CurrentDigest = ct.RepoDigests
			var indexDigest, childDigest string
			var err error
			if isHaveUpdate {
				// 获取远端 digest：indexDigest 为清单索引，多架构镜像；childDigest 为匹配当前平台的子 manifest
				indexDigest, childDigest, err = s.registry.GetRemoteDigests(ctx, ct.Image, isUserCache)
			} else {
				indexDigest, err = s.registry.GetRemoteDigestByCache(ctx, ct.Image)
			}
			if err != nil {
				logger.Logger.Error("get remote digest", logger.ZapErr(err))
				st.Status = "Error"
				st.SkipReason = "registry: " + err.Error()
				result[j.idx] = st
				continue
			}
			chosen := indexDigest
			if chosen == "" {
				chosen = childDigest
			}
			st.RemoteDigest = chosen
			if len(st.CurrentDigest) == 0 || (st.RemoteDigest != "" && !compareDigests(st.CurrentDigest, st.RemoteDigest)) {
				st.Status = "UpdateAvailable"
			} else {
				st.Status = "UpToDate"
			}
			result[j.idx] = st
		}
		done <- struct{}{}
	}

	// 启动固定数量的 worker，限制并发请求的数量
	for w := 0; w < concurrency; w++ {
		go worker()
	}
	// 投递扫描任务；支持 ctx 取消
	go func() {
		for i := range containers {
			select {
			case jobs <- job{idx: i}:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	// 等待所有 worker 退出（消费完 jobs）
	for w := 0; w < concurrency; w++ {
		<-done
	}
	return result, nil
}

func compareDigests(currentDigests []string, remoteDigest string) bool {
	for _, d := range currentDigests {
		localDigest := strings.Split(d, "@")[1]
		if localDigest == remoteDigest {
			return true
		}
	}
	return false
}
