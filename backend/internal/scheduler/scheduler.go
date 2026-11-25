package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/notificationmanager"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/updater"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler 负责周期扫描与按 cron 自动更新
type Scheduler struct {
	logger              *zap.Logger
	scanner             *scanner.Scanner
	updater             *updater.Updater
	notificationManager *notificationmanager.Manager

	mu         sync.Mutex
	runMu      sync.Mutex
	stopped    bool
	cancel     context.CancelFunc
	stopCtx    context.Context
	stopCancel context.CancelFunc
	cron       *cron.Cron
	entryID    cron.EntryID
}

func New(logger *zap.Logger, sc *scanner.Scanner, up *updater.Updater, nm *notificationmanager.Manager) *Scheduler {
	stopCtx, stopCancel := context.WithCancel(context.Background())
	return &Scheduler{
		logger:              logger,
		scanner:             sc,
		updater:             up,
		notificationManager: nm,
		stopCtx:             stopCtx,
		stopCancel:          stopCancel,
	}
}

func (s *Scheduler) ensureStopCtxLocked() context.Context {
	if s.stopCtx == nil {
		s.stopCtx, s.stopCancel = context.WithCancel(context.Background())
	}
	return s.stopCtx
}

// Start 启动调度器；同时兼容 5 段与 6 段（含秒）Cron 表达式，并跳过未完成的重叠执行。
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}

	cfg := config.Get()
	if cfg.Scan.Cron == "" {
		s.removeTaskLocked()
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.logger.Info("未配置 cron 表达式，调度器将不启动")
		return
	}

	s.removeTaskLocked()

	if s.cron == nil {
		parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		s.cron = cron.New(
			cron.WithParser(parser),
			cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)),
		)
		s.cron.Start()
		s.logger.Info("cron 调度器已启动")
	}

	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	taskName := "scan-and-update"
	s.logger.Info("开始添加 cron 任务",
		zap.String("taskName", taskName),
		zap.String("cron", cfg.Scan.Cron))

	entryID, err := s.cron.AddFunc(cfg.Scan.Cron, func() {
		s.RunScanAndUpdate(ctx)
	})
	if err != nil {
		s.logger.Error("添加 cron 任务失败",
			zap.String("taskName", taskName),
			zap.Error(err))
		return
	}

	s.entryID = entryID
	s.logger.Info("cron 任务添加成功",
		zap.String("taskName", taskName),
		zap.Int("entryID", int(entryID)))
}

// Stop 停止当前定时任务并取消运行中扫描/更新上下文
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.removeTaskLocked()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Scheduler) removeTaskLocked() {
	if s.cron != nil && s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.logger.Info("已移除定时任务", zap.Int("entryID", int(s.entryID)))
		s.entryID = 0
	}
}

// RemoveTask 移除当前的定时任务
func (s *Scheduler) RemoveTask() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeTaskLocked()
}

// StopCron 停止并等待 cron 调度器以及进行中的扫描/更新任务彻底退出
func (s *Scheduler) StopCron() {
	s.mu.Lock()
	s.stopped = true
	c := s.cron
	cancel := s.cancel
	stopCancel := s.stopCancel
	s.cron = nil
	s.entryID = 0
	s.cancel = nil
	s.stopCancel = nil
	s.mu.Unlock()

	if stopCancel != nil {
		stopCancel()
	}
	if cancel != nil {
		cancel()
	}
	if c != nil {
		stopCtx := c.Stop()
		<-stopCtx.Done()
		s.logger.Info("cron 调度器已停止")
	}
	// 等待经 HTTP update-all 或 cron 触发的在途 RunScanAndUpdate 完整退出（避免容器更新/回滚中途被 shutdown 中断）
	s.runMu.Lock()
	s.runMu.Unlock()
}

// Restart 重启调度器，重新读取配置
func (s *Scheduler) Restart() {
	s.logger.Info("重启调度器")
	s.Start()
}

// IsRunning 检查调度器是否正在运行
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cron != nil && s.entryID != 0
}

// GetTaskInfo 获取当前任务信息
func (s *Scheduler) GetTaskInfo() (bool, int, string) {
	s.mu.Lock()
	running := s.cron != nil && s.entryID != 0
	id := int(s.entryID)
	s.mu.Unlock()

	if !running {
		return false, 0, ""
	}
	cfg := config.Get()
	return true, id, cfg.Scan.Cron
}

func (s *Scheduler) RunScanAndUpdate(ctx context.Context) {
	s.mu.Lock()
	stopped := s.stopped
	stopCtx := s.ensureStopCtxLocked()
	s.mu.Unlock()
	if stopped || stopCtx.Err() != nil {
		return
	}

	if !s.runMu.TryLock() {
		s.logger.Info("上一次扫描更新任务仍在运行，跳过本次触发")
		return
	}
	defer s.runMu.Unlock()

	runCtx, runCancel := context.WithCancel(ctx)
	stopDone := make(chan struct{})
	go func() {
		select {
		case <-stopCtx.Done():
			runCancel()
		case <-runCtx.Done():
		case <-stopDone:
		}
	}()
	defer func() {
		close(stopDone)
		runCancel()
	}()
	if runCtx.Err() != nil {
		return
	}

	s.logger.Info("开始执行扫描更新任务")
	cfg := config.Get()
	includeStopped := cfg.Docker.IncludeStopped
	conc := cfg.Scan.Concurrency
	statuses, err := s.scanner.ScanOnce(runCtx, includeStopped, conc, true, true)
	if err != nil {
		s.logger.Error("scan failed", zap.Error(err))
		return
	}
	s.logger.Info("扫描更新任务完成")
	s.logger.Debug("扫描更新结果", zap.Any("statuses", statuses))

	if s.notificationManager != nil {
		var updateAvailableContainers []scanner.ContainerStatus
		for _, st := range statuses {
			if st.Status == "UpdateAvailable" && !st.Skipped {
				updateAvailableContainers = append(updateAvailableContainers, st)
			}
		}
		if len(updateAvailableContainers) > 0 {
			if err := s.notificationManager.NotifyUpdateAvailable(runCtx, updateAvailableContainers); err != nil {
				s.logger.Error("发送更新可用通知失败", zap.Error(err))
			}
		}
	}

	updateStatuses := make([]scanner.ContainerStatus, 0)
	for _, st := range statuses {
		if st.Skipped || st.SkippedUpdate || st.Status != "UpdateAvailable" {
			continue
		}
		updateStatuses = append(updateStatuses, st)
	}
	if len(updateStatuses) == 0 {
		if cfg.Scan.IsUpdate {
			s.logger.Info("没有需要更新的容器")
		} else {
			s.logger.Warn("没有需要更新的容器, 更新开关关闭")
		}
		return
	}

	if !cfg.Scan.IsUpdate {
		return
	}

	s.logger.Info("开始执行批量更新任务")
	for _, st := range updateStatuses {
		if runCtx.Err() != nil {
			break
		}
		uctx, cancel := context.WithTimeout(runCtx, 10*time.Minute)
		s.logger.Info(fmt.Sprintf("开始执行更新任务: %s", st.Name))
		if _, err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
			cancel()
			if runCtx.Err() != nil {
				s.logger.Info(fmt.Sprintf("更新任务随调度器停止而中断: %s", st.Name))
				break
			}
			s.logger.Error(fmt.Sprintf("更新任务失败: %s", st.Name), zap.Error(err))
			if s.notificationManager != nil {
				if notifyErr := s.notificationManager.NotifyUpdateFailed(runCtx, st.Name, st.Image, err.Error()); notifyErr != nil {
					s.logger.Error("发送更新失败通知失败", zap.Error(notifyErr))
				}
			}
			continue
		}
		cancel()
		s.logger.Info(fmt.Sprintf("更新任务完成: %s", st.Name))
		if s.notificationManager != nil {
			if notifyErr := s.notificationManager.NotifyUpdateSuccess(runCtx, st.Name, st.Image); notifyErr != nil {
				s.logger.Error("发送更新成功通知失败", zap.Error(notifyErr))
			}
		}
	}
	s.logger.Info("批量更新任务完成")
}
