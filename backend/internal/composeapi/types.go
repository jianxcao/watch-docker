package composeapi

import (
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
)

// MessageType 定义消息类型
type MessageType int

const (
	MessageTypeLog MessageType = iota
	MessageTypeError
	MessageTypeStatus
	MessageTypeComplete
)

// StreamMessage 表示流式输出的消息
type StreamMessage struct {
	Type    MessageType // 消息类型
	Content string      // 日志内容
	Error   error       // 错误信息（仅当 Type 为 MessageTypeError 时）
}

// 复用 composecli 中的类型定义
type (
	ComposeProject = composecli.ComposeProject
	StackStatus    = composecli.StackStatus
)

// 复用状态常量
var (
	StatusRunning      = composecli.StatusRunning
	StatusExited       = composecli.StatusExited
	StatusDraft        = composecli.StatusDraft
	StatusPartial      = composecli.StatusPartial
	StatusCreatedStack = composecli.StatusCreatedStack
	StatusUnknown      = composecli.StatusUnknown
)
