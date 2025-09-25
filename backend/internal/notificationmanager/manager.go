package notificationmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/notify"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"

	"go.uber.org/zap"
)

// Manager 通知管理器
type Manager struct {
	notifier      *notify.Notifier
	history       *NotificationHistory
	historyPath   string
	pendingEvents []ContainerNotification
	mu            sync.RWMutex
	batchTimer    *time.Timer
	batchDelay    time.Duration // 批量延迟时间，用于合并通知
}

// New 创建新的通知管理器
func New(notifier *notify.Notifier, historyPath string) *Manager {
	if historyPath == "" {
		historyPath = "/tmp/watch-docker-notification-history.json"
	}

	m := &Manager{
		notifier:      notifier,
		historyPath:   historyPath,
		pendingEvents: make([]ContainerNotification, 0),
		batchDelay:    60 * time.Second, // 30秒内的通知会被合并
	}

	m.loadHistory()
	return m
}

// SetBatchDelay 设置批量延迟时间
func (m *Manager) SetBatchDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchDelay = delay
}

// NotifyUpdateAvailable 通知有更新可用
func (m *Manager) NotifyUpdateAvailable(ctx context.Context, containers []scanner.ContainerStatus) error {
	if len(containers) == 0 {
		return nil
	}

	var validContainers []ContainerNotification
	for _, cs := range containers {
		if cs.Status != "UpdateAvailable" {
			continue
		}

		// 检查去重
		if m.shouldSkipNotification(cs.Name, cs.Image, cs.RemoteDigest, EventUpdateAvailable) {
			logger.Logger.Debug("跳过重复通知",
				zap.String("container", cs.Name),
				zap.String("image", cs.Image),
				zap.String("digest", cs.RemoteDigest))
			continue
		}

		var cn ContainerNotification
		cn.FromContainerStatus(cs, EventUpdateAvailable)
		validContainers = append(validContainers, cn)

		// 标记为已通知
		m.markAsNotified(cs.Name, cs.Image, cs.RemoteDigest, EventUpdateAvailable)
	}

	if len(validContainers) == 0 {
		return nil
	}

	// 添加到待处理事件队列
	m.mu.Lock()
	m.pendingEvents = append(m.pendingEvents, validContainers...)
	m.scheduleFlush()
	m.mu.Unlock()

	return nil
}

// NotifyUpdateSuccess 通知更新成功
func (m *Manager) NotifyUpdateSuccess(ctx context.Context, containerName, image string) error {
	cn := ContainerNotification{
		Type:          EventUpdateSuccess,
		ContainerName: containerName,
		Image:         image,
		Timestamp:     time.Now(),
	}

	m.mu.Lock()
	m.pendingEvents = append(m.pendingEvents, cn)
	m.scheduleFlush()
	m.mu.Unlock()

	return nil
}

// NotifyUpdateFailed 通知更新失败
func (m *Manager) NotifyUpdateFailed(ctx context.Context, containerName, image, errorMsg string) error {
	cn := ContainerNotification{
		Type:          EventUpdateFailed,
		ContainerName: containerName,
		Image:         image,
		Error:         errorMsg,
		Timestamp:     time.Now(),
	}

	m.mu.Lock()
	m.pendingEvents = append(m.pendingEvents, cn)
	m.scheduleFlush()
	m.mu.Unlock()

	return nil
}

// scheduleFlush 调度批量发送（需要在持有锁的情况下调用）
func (m *Manager) scheduleFlush() {
	if m.batchTimer != nil {
		m.batchTimer.Stop()
	}

	m.batchTimer = time.AfterFunc(m.batchDelay, func() {
		m.flushPendingEvents()
	})
}

// flushPendingEvents 立即发送所有待处理的事件
func (m *Manager) flushPendingEvents() {
	m.mu.Lock()
	if len(m.pendingEvents) == 0 {
		m.mu.Unlock()
		return
	}

	events := make([]ContainerNotification, len(m.pendingEvents))
	copy(events, m.pendingEvents)
	m.pendingEvents = m.pendingEvents[:0] // 清空
	m.mu.Unlock()

	// 按类型分组
	batch := m.groupEventsByType(events)

	// 发送通知
	if err := m.sendBatchNotification(context.Background(), batch); err != nil {
		logger.Logger.Error("发送批量通知失败", zap.Error(err))
	}
}

// groupEventsByType 按事件类型分组
func (m *Manager) groupEventsByType(events []ContainerNotification) NotificationBatch {
	batch := NotificationBatch{
		Timestamp: time.Now(),
	}

	for _, event := range events {
		switch event.Type {
		case EventUpdateAvailable:
			batch.UpdateAvailable = append(batch.UpdateAvailable, event)
		case EventUpdateSuccess:
			batch.UpdateSuccess = append(batch.UpdateSuccess, event)
		case EventUpdateFailed:
			batch.UpdateFailed = append(batch.UpdateFailed, event)
		}
	}

	return batch
}

// sendBatchNotification 发送批量通知
func (m *Manager) sendBatchNotification(ctx context.Context, batch NotificationBatch) error {
	if len(batch.UpdateAvailable) == 0 && len(batch.UpdateSuccess) == 0 && len(batch.UpdateFailed) == 0 {
		return nil
	}

	title, content := m.formatNotificationContent(batch)

	return m.notifier.Send(ctx, title, content, "", "")
}

// formatNotificationContent 格式化通知内容
func (m *Manager) formatNotificationContent(batch NotificationBatch) (string, string) {
	var titleParts []string
	var contentLines []string

	// 有更新可用
	if len(batch.UpdateAvailable) > 0 {
		if len(batch.UpdateAvailable) == 1 {
			titleParts = append(titleParts, "1个容器有更新")
		} else {
			titleParts = append(titleParts, fmt.Sprintf("%d个容器有更新", len(batch.UpdateAvailable)))
		}

		contentLines = append(contentLines, "📦 有更新可用的容器:")
		for _, event := range batch.UpdateAvailable {
			contentLines = append(contentLines,
				fmt.Sprintf("  • %s (%s)", event.ContainerName, event.Image))
		}
		contentLines = append(contentLines, "")
	}

	// 更新成功
	if len(batch.UpdateSuccess) > 0 {
		if len(batch.UpdateSuccess) == 1 {
			titleParts = append(titleParts, "1个容器更新成功")
		} else {
			titleParts = append(titleParts, fmt.Sprintf("%d个容器更新成功", len(batch.UpdateSuccess)))
		}

		contentLines = append(contentLines, "✅ 更新成功的容器:")
		for _, event := range batch.UpdateSuccess {
			contentLines = append(contentLines,
				fmt.Sprintf("  • %s (%s)", event.ContainerName, event.Image))
		}
		contentLines = append(contentLines, "")
	}

	// 更新失败
	if len(batch.UpdateFailed) > 0 {
		if len(batch.UpdateFailed) == 1 {
			titleParts = append(titleParts, "1个容器更新失败")
		} else {
			titleParts = append(titleParts, fmt.Sprintf("%d个容器更新失败", len(batch.UpdateFailed)))
		}

		contentLines = append(contentLines, "❌ 更新失败的容器:")
		for _, event := range batch.UpdateFailed {
			errorInfo := ""
			if event.Error != "" {
				errorInfo = fmt.Sprintf(" (错误: %s)", event.Error)
			}
			contentLines = append(contentLines,
				fmt.Sprintf("  • %s (%s)%s", event.ContainerName, event.Image, errorInfo))
		}
	}

	title := strings.Join(titleParts, "，")
	if title == "" {
		title = "Docker 容器状态更新"
	}

	content := strings.Join(contentLines, "\n")
	if content != "" {
		content += fmt.Sprintf("\n⏰ 通知时间: %s", batch.Timestamp.Format("2006-01-02 15:04:05"))
	}

	return title, content
}

// shouldSkipNotification 检查是否应该跳过通知（去重逻辑）
func (m *Manager) shouldSkipNotification(containerName, image, digest string, eventType NotificationEventType) bool {
	// 只对 UpdateAvailable 事件进行去重
	if eventType != EventUpdateAvailable {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	today := time.Now().Format("2006-01-02")
	if m.history.Date != today {
		// 日期变了，重置历史记录
		return false
	}

	key := fmt.Sprintf("%s|%s|%s|%s", containerName, image, digest, today)
	return m.history.SentToday[key]
}

// markAsNotified 标记为已通知
func (m *Manager) markAsNotified(containerName, image, digest string, eventType NotificationEventType) {
	// 只对 UpdateAvailable 事件进行标记
	if eventType != EventUpdateAvailable {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	today := time.Now().Format("2006-01-02")

	// 检查日期是否变化，如果变化了就清理过期记录
	if m.history.Date != today {
		logger.Logger.Debug("日期变更，清理过期通知历史",
			zap.String("old_date", m.history.Date),
			zap.String("today", today))
		m.history = &NotificationHistory{
			SentToday: make(map[string]bool),
			Date:      today,
		}
	}

	key := fmt.Sprintf("%s|%s|%s|%s", containerName, image, digest, today)
	m.history.SentToday[key] = true

	// 保存历史记录
	m.saveHistory()
}

// loadHistory 加载通知历史
func (m *Manager) loadHistory() {
	today := time.Now().Format("2006-01-02")

	data, err := os.ReadFile(m.historyPath)
	if err != nil {
		logger.Logger.Debug("无法读取通知历史文件", zap.String("path", m.historyPath), zap.Error(err))
		m.history = &NotificationHistory{
			SentToday: make(map[string]bool),
			Date:      today,
		}
		return
	}

	var history NotificationHistory
	if err := json.Unmarshal(data, &history); err != nil {
		logger.Logger.Error("解析通知历史文件失败", zap.Error(err))
		m.history = &NotificationHistory{
			SentToday: make(map[string]bool),
			Date:      today,
		}
		return
	}

	// 只保留当天的历史记录，过期的直接丢弃
	if history.Date != today {
		logger.Logger.Debug("历史记录已过期，重置为新的一天",
			zap.String("old_date", history.Date),
			zap.String("today", today))
		m.history = &NotificationHistory{
			SentToday: make(map[string]bool),
			Date:      today,
		}
		// 立即保存新的空记录，覆盖过期文件
		m.saveHistory()
	} else {
		m.history = &history
	}
}

// saveHistory 保存通知历史
func (m *Manager) saveHistory() {
	// 确保目录存在
	dir := filepath.Dir(m.historyPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Logger.Error("创建历史文件目录失败", zap.String("dir", dir), zap.Error(err))
		return
	}

	data, err := json.MarshalIndent(m.history, "", "  ")
	if err != nil {
		logger.Logger.Error("序列化通知历史失败", zap.Error(err))
		return
	}

	if err := os.WriteFile(m.historyPath, data, 0644); err != nil {
		logger.Logger.Error("保存通知历史文件失败", zap.String("path", m.historyPath), zap.Error(err))
	}
}

// Flush 立即发送所有待处理的通知
func (m *Manager) Flush() {
	m.flushPendingEvents()
}

// Close 关闭通知管理器，发送所有待处理的通知
func (m *Manager) Close() {
	m.mu.Lock()
	if m.batchTimer != nil {
		m.batchTimer.Stop()
		m.batchTimer = nil
	}
	m.mu.Unlock()

	// 发送所有待处理的通知
	m.flushPendingEvents()
}

// GetHistoryStats 获取历史记录统计信息（用于监控和调试）
func (m *Manager) GetHistoryStats() (date string, count int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.history == nil {
		return "", 0
	}

	return m.history.Date, len(m.history.SentToday)
}
