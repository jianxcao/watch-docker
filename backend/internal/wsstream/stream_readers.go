package wsstream

import (
	"context"
	"io"
	"sync"
)

// ByteStreamReader 直接读取数据块，不保证边界
// 适用于日志流等不需要消息边界的场景
type ByteStreamReader struct {
	reader    io.ReadCloser
	buffer    []byte
	closeOnce sync.Once
	closeErr  error
}

// NewByteStreamReader 创建字节流读取器
func NewByteStreamReader(reader io.ReadCloser) *ByteStreamReader {
	return &ByteStreamReader{
		reader: reader,
		buffer: make([]byte, 4096),
	}
}

// Read 读取一块数据
func (r *ByteStreamReader) Read(ctx context.Context) ([]byte, error) {
	n, err := r.reader.Read(r.buffer)
	if n > 0 {
		// 复制数据避免被下次读取覆盖
		data := make([]byte, n)
		copy(data, r.buffer[:n])
		return data, nil
	}
	return nil, err
}

// Close 关闭读取器（幂等，可多次调用）
func (r *ByteStreamReader) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.reader.Close()
	})
	return r.closeErr
}

// ChannelStreamReader 从 channel 读取完整消息
// 输入 channel 中的每个值都是一条完整的消息，无需额外处理
// 支持泛型：可处理 []byte 或 string
type ChannelStreamReader[T MessageType] struct {
	inputChan chan T // 数据源写入完整消息的 channel
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// NewChannelStreamReader 创建基于 channel 的 Reader
// inputChan: 数据源往这个 channel 写入完整的消息
func NewChannelStreamReader[T MessageType](inputChan chan T) *ChannelStreamReader[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelStreamReader[T]{
		inputChan: inputChan,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Read 从 channel 读取下一条完整消息
func (r *ChannelStreamReader[T]) Read(ctx context.Context) (T, error) {
	select {
	case message, ok := <-r.inputChan:
		if !ok {
			// channel 已关闭
			var zero T
			return zero, io.EOF
		}
		return message, nil

	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()

	case <-r.ctx.Done():
		var zero T
		return zero, io.EOF
	}
}

// Close 关闭读取器
func (r *ChannelStreamReader[T]) Close() error {
	r.closeOnce.Do(func() {
		r.cancel()
	})
	return nil
}
