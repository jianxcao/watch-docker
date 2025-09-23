package scheduler

import (
	"context"
	"fmt"
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
		if cfg.Scan.Cron != "" {
			c := cron.New(cron.WithSeconds())
			s.logger.Info("开始添加 cron 任务", zap.String("cron", cfg.Scan.Cron))
			_, err := c.AddFunc(cfg.Scan.Cron, func() { s.runScanAndUpdate(ctx) })
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
	}()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Scheduler) runScanAndUpdate(ctx context.Context) {
	s.logger.Info("开始执行扫描更新任务")
	cfg := config.Get()
	includeStopped := cfg.Docker.IncludeStopped
	conc := cfg.Scan.Concurrency
	statuses, err := s.scanner.ScanOnce(ctx, includeStopped, conc)
	if err != nil {
		s.logger.Error("scan failed", zap.Error(err))
		return
	}
	s.logger.Info("扫描更新任务完成", zap.Any("statuses", statuses))
	if !cfg.Scan.IsUpdate {
		return
	}

	var updateStatuses []scanner.ContainerStatus = make([]scanner.ContainerStatus, 0)
	for _, st := range statuses {
		if st.Skipped || st.Status != "UpdateAvailable" {
			continue
		}
		updateStatuses = append(updateStatuses, st)
	}
	if len(updateStatuses) == 0 {
		s.logger.Info("没有需要更新的容器")
		return
	}
	s.logger.Info("开始执行更新任务")
	for _, st := range updateStatuses {
		uctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		s.logger.Info(fmt.Sprintf("开始执行更新任务: %s", st.Name))
		if err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
			s.logger.Error(fmt.Sprintf("更新任务失败: %s", st.Name), zap.Error(err))
		}
		s.logger.Info(fmt.Sprintf("更新任务完成: %s", st.Name))
		cancel()
	}
	s.logger.Info("更新任务完成")
}
