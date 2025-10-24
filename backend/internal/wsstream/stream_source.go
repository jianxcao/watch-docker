package wsstream

import (
	"context"
)

// MessageType 定义消息类型约束
type MessageType interface {
	~[]byte | ~string
}

// StreamReader 定义消息读取器接口（泛型）
type StreamReader[T MessageType] interface {
	// Read 读取下一条完整消息
	// 返回 io.EOF 表示流结束
	Read(ctx context.Context) (T, error)

	// Close 关闭读取器
	Close() error
}

// StreamSource 定义流式数据源的接口（泛型版本）
type StreamSource[T MessageType] interface {
	// Start 启动数据源，返回消息读取器
	Start(ctx context.Context) (StreamReader[T], error)

	// Stop 停止数据源，清理资源
	Stop() error

	// GetKey 获取数据源的唯一标识
	// 相同 key 的客户端会共享同一个数据源
	GetKey() string
}
