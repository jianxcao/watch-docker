package scheduler

import (
	"context"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/updater"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler 负责周期扫描与按 cron 自动更新
type Scheduler struct {
	logger  *zap.Logger
	scanner *scanner.Scanner
	updater *updater.Updater

	cancel context.CancelFunc
}

func New(logger *zap.Logger, sc *scanner.Scanner, up *updater.Updater) *Scheduler {
	return &Scheduler{logger: logger, scanner: sc, updater: up}
}

// Start 启动调度器：优先使用 cron；未配置 cron 时退回到 interval 定时器。
func (s *Scheduler) Start() {
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// 单一路径：若配置了 cron 则使用 cron；否则使用 interval 定时器
	go func() {
		cfg := config.Get()
		if cfg.Update.AutoUpdateCron != "" {
			c := cron.New(cron.WithSeconds())
			_, err := c.AddFunc(cfg.Update.AutoUpdateCron, func() { s.runScanAndUpdate(ctx) })
			if err != nil {
				s.logger.Error("cron add failed", zap.Error(err))
				return
			}
			c.Start()
			<-ctx.Done()
			ctx2 := c.Stop()
			<-ctx2.Done()
			return
		}

		// 无 cron：仅按 interval 扫描状态，不执行自动更新
		for {
			cfg := config.Get()
			interval := cfg.Scan.Interval
			if interval <= 0 {
				interval = 10 * time.Minute
			}
			s.runScanOnly(ctx)
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// runScanAndMaybeUpdate 执行扫描；是否自动更新由配置决定（Update.Enabled）。
func (s *Scheduler) runScanOnly(ctx context.Context) {
	cfg := config.Get()
	includeStopped := cfg.Docker.IncludeStopped
	conc := cfg.Scan.Concurrency
	statuses, err := s.scanner.ScanOnce(ctx, includeStopped, conc)
	if err != nil {
		s.logger.Error("scan failed", zap.Error(err))
		return
	}
	_ = statuses
}

func (s *Scheduler) runScanAndUpdate(ctx context.Context) {
	cfg := config.Get()
	includeStopped := cfg.Docker.IncludeStopped
	conc := cfg.Scan.Concurrency
	statuses, err := s.scanner.ScanOnce(ctx, includeStopped, conc)
	if err != nil {
		s.logger.Error("scan failed", zap.Error(err))
		return
	}
	if !cfg.Update.Enabled {
		return
	}
	for _, st := range statuses {
		if st.Skipped || st.Status != "UpdateAvailable" {
			continue
		}
		uctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		if err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
			s.logger.Error("auto update failed", zap.String("container", st.Name), zap.Error(err))
		}
		cancel()
	}
}
