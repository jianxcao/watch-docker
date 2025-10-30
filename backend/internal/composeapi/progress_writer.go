package composeapi

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/compose/v2/pkg/api"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
)

// ChannelWriter 实现一个将输出发送到 channel 的 Writer
type ChannelWriter struct {
	ch     chan<- StreamMessage
	ctx    context.Context
	closed bool
}

// NewChannelWriter 创建一个新的 ChannelWriter
func NewChannelWriter(ctx context.Context, ch chan<- StreamMessage) *ChannelWriter {
	return &ChannelWriter{
		ch:     ch,
		ctx:    ctx,
		closed: false,
	}
}

// Write 实现 io.Writer 接口
func (w *ChannelWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}

	// 检查 context 是否已取消
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
	}

	content := string(p)
	if content != "" {
		select {
		case w.ch <- StreamMessage{
			Type:    MessageTypeLog,
			Content: content,
		}:
		case <-w.ctx.Done():
			return 0, w.ctx.Err()
		}
	}

	return len(p), nil
}

// Close 关闭 writer
func (w *ChannelWriter) Close() error {
	w.closed = true
	return nil
}

// Event 实现 compose 的事件处理接口
func (w *ChannelWriter) Event(event api.Event) {
	if w.closed {
		return
	}

	// 根据 compose v2 的 Event 结构构造消息
	msg := fmt.Sprintf("%s\n", event)

	select {
	case w.ch <- StreamMessage{
		Type:    MessageTypeLog,
		Content: msg,
	}:
	case <-w.ctx.Done():
		logger.Logger.Debug("context cancelled while sending event")
	}
}

// Events 批量处理事件
func (w *ChannelWriter) Events(events []api.Event) {
	for _, event := range events {
		w.Event(event)
	}
}

// TailMsgf 处理尾部消息
func (w *ChannelWriter) TailMsgf(msg string, args ...interface{}) {
	if w.closed {
		return
	}

	content := fmt.Sprintf(msg+"\n", args...)
	select {
	case w.ch <- StreamMessage{
		Type:    MessageTypeLog,
		Content: content,
	}:
	case <-w.ctx.Done():
	}
}

// HasMore 指示是否有更多内容
func (w *ChannelWriter) HasMore(more bool) {
	// 用于指示是否有更多内容，这里我们只记录日志
	if more {
		logger.Logger.Debug("progress writer has more content")
	}
}

// Dry 返回是否为 dry-run 模式
func (w *ChannelWriter) Dry() bool {
	return false
}

// Stop 停止写入
func (w *ChannelWriter) Stop() {
	w.closed = true
}

