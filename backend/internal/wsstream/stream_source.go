package wsstream

import (
	"context"
	"io"
)

// StreamSource 定义流式数据源的接口
// 任何需要通过 WebSocket 广播的数据源都应实现此接口
type StreamSource interface {
	// Start 启动数据源，返回可读取数据的 Reader
	// ctx 用于控制数据源的生命周期
	Start(ctx context.Context) (io.ReadCloser, error)

	// Stop 停止数据源，清理资源
	Stop() error

	// GetKey 获取数据源的唯一标识
	// 相同 key 的客户端会共享同一个数据源
	GetKey() string
}
