package notificationmanager

import (
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/scanner"
)

// NotificationEventType 定义通知事件类型
type NotificationEventType string

const (
	EventUpdateAvailable NotificationEventType = "update_available"
	EventUpdateSuccess   NotificationEventType = "update_success"
	EventUpdateFailed    NotificationEventType = "update_failed"
)

// ContainerNotification 表示一个容器的通知事件
type ContainerNotification struct {
	Type          NotificationEventType `json:"type"`
	ContainerID   string                `json:"container_id"`
	ContainerName string                `json:"container_name"`
	Image         string                `json:"image"`
	CurrentDigest string                `json:"current_digest"`
	RemoteDigest  string                `json:"remote_digest"`
	Timestamp     time.Time             `json:"timestamp"`
	Error         string                `json:"error,omitempty"` // 只有在失败时才有
}

// NotificationBatch 表示一批通知事件
type NotificationBatch struct {
	UpdateAvailable []ContainerNotification `json:"update_available"`
	UpdateSuccess   []ContainerNotification `json:"update_success"`
	UpdateFailed    []ContainerNotification `json:"update_failed"`
	Timestamp       time.Time               `json:"timestamp"`
}

// DeduplicationKey 用于去重的键
type DeduplicationKey struct {
	ContainerName string `json:"container_name"`
	Image         string `json:"image"`
	Digest        string `json:"digest"`
	Date          string `json:"date"` // YYYY-MM-DD 格式
}

// NotificationHistory 记录通知历史，用于去重
// 只保存当天的记录，过期的记录会被自动清理
type NotificationHistory struct {
	SentToday map[string]bool `json:"sent_today"` // key是容器名|镜像|摘要|日期的组合，只存储当天的记录
	Date      string          `json:"date"`       // YYYY-MM-DD 格式，用于判断记录是否过期
}

// FromContainerStatus 从 ContainerStatus 创建 ContainerNotification
func (cn *ContainerNotification) FromContainerStatus(cs scanner.ContainerStatus, eventType NotificationEventType) {
	cn.Type = eventType
	cn.ContainerID = cs.ID
	cn.ContainerName = cs.Name
	cn.Image = cs.Image
	cn.CurrentDigest = cs.CurrentDigest
	cn.RemoteDigest = cs.RemoteDigest
	cn.Timestamp = time.Now()
}
