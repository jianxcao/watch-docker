package scheduler

import (
	"context"
	"fmt"
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

	cancel  context.CancelFunc
	cron    *cron.Cron
	entryID cron.EntryID // 任务ID，用于管理和移除任务
}

func New(logger *zap.Logger, sc *scanner.Scanner, up *updater.Updater, nm *notificationmanager.Manager) *Scheduler {
	return &Scheduler{
		logger:              logger,
		scanner:             sc,
		updater:             up,
		notificationManager: nm,
	}
}

// Start 启动调度器：优先使用 cron；未配置 cron 时退回到 interval 定时器。
func (s *Scheduler) Start() {
	cfg := config.Get()
	if cfg.Scan.Cron == "" {
		s.logger.Info("未配置 cron 表达式，调度器将不启动")
		return
	}

	// 移除已存在的任务
	s.RemoveTask()

	// 创建或复用 cron 实例
	if s.cron == nil {
		s.cron = cron.New(cron.WithSeconds())
		s.cron.Start()
		s.logger.Info("cron 调度器已启动")
	}

	// 设置上下文
	if s.cancel != nil {
		s.cancel() // 取消之前的 context
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// 添加新任务
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

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.RemoveTask()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// RemoveTask 移除当前的定时任务
func (s *Scheduler) RemoveTask() {
	if s.cron != nil && s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.logger.Info("已移除定时任务", zap.Int("entryID", int(s.entryID)))
		s.entryID = 0
	}
}

// StopCron 停止并清理 cron 调度器
func (s *Scheduler) StopCron() {
	if s.cron != nil {
		ctx := s.cron.Stop()
		<-ctx.Done()
		s.cron = nil
		s.entryID = 0
		s.logger.Info("cron 调度器已停止")
	}
}

// Restart 重启调度器，重新读取配置
func (s *Scheduler) Restart() {
	s.logger.Info("重启调度器")
	s.Start() // Start 方法会自动移除旧任务并添加新任务
}

// IsRunning 检查调度器是否正在运行
func (s *Scheduler) IsRunning() bool {
	return s.cron != nil && s.entryID != 0
}

// GetTaskInfo 获取当前任务信息
func (s *Scheduler) GetTaskInfo() (bool, int, string) {
	if s.cron == nil || s.entryID == 0 {
		return false, 0, ""
	}

	cfg := config.Get()
	return true, int(s.entryID), cfg.Scan.Cron
}

func (s *Scheduler) RunScanAndUpdate(ctx context.Context) {
	s.logger.Info("开始执行扫描更新任务")
	cfg := config.Get()
	includeStopped := cfg.Docker.IncludeStopped
	conc := cfg.Scan.Concurrency
	statuses, err := s.scanner.ScanOnce(ctx, includeStopped, conc, true, true)
	if err != nil {
		s.logger.Error("scan failed", zap.Error(err))
		return
	}
	s.logger.Info("扫描更新任务完成")
	s.logger.Debug("扫描更新结果", zap.Any("statuses", statuses))

	// 通知有更新可用的容器
	if s.notificationManager != nil {
		var updateAvailableContainers []scanner.ContainerStatus
		for _, st := range statuses {
			if st.Status == "UpdateAvailable" && !st.Skipped {
				updateAvailableContainers = append(updateAvailableContainers, st)
			}
		}
		if len(updateAvailableContainers) > 0 {
			if err := s.notificationManager.NotifyUpdateAvailable(ctx, updateAvailableContainers); err != nil {
				s.logger.Error("发送更新可用通知失败", zap.Error(err))
			}
		}
	}

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
	s.logger.Info("开始执行批量更新任务")
	for _, st := range updateStatuses {
		uctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		s.logger.Info(fmt.Sprintf("开始执行更新任务: %s", st.Name))
		if err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
			s.logger.Error(fmt.Sprintf("更新任务失败: %s", st.Name), zap.Error(err))
			// 通知更新失败
			if s.notificationManager != nil {
				if notifyErr := s.notificationManager.NotifyUpdateFailed(ctx, st.Name, st.Image, err.Error()); notifyErr != nil {
					s.logger.Error("发送更新失败通知失败", zap.Error(notifyErr))
				}
			}
		} else {
			s.logger.Info(fmt.Sprintf("更新任务完成: %s", st.Name))
			// 通知更新成功
			if s.notificationManager != nil {
				if notifyErr := s.notificationManager.NotifyUpdateSuccess(ctx, st.Name, st.Image); notifyErr != nil {
					s.logger.Error("发送更新成功通知失败", zap.Error(notifyErr))
				}
			}
		}
		cancel()
	}
	s.logger.Info("批量更新任务完成")
}
