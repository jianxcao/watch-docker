package logger

import (
	"sync"
)

// 日志广播实现，供 SSE 等实时订阅使用
const bufferSize = 500

var (
	subscribersMu sync.Mutex
	subscribers   = make(map[chan string]struct{})
	logBuffer     []string
)

// Subscribe 创建一个新的日志订阅通道，并将缓冲区日志发送给新订阅者
func Subscribe() chan string {
	ch := make(chan string, bufferSize+100)

	// 先把现有缓冲区内容复制出来，避免长时间持锁
	subscribersMu.Lock()
	snapshot := append([]string(nil), logBuffer...)
	subscribers[ch] = struct{}{}
	subscribersMu.Unlock()

	// 异步把历史日志推送给新订阅者，避免阻塞调用方
	go func() {
		for _, item := range snapshot {
			ch <- item
		}
	}()

	return ch
}

// Unsubscribe 取消订阅并关闭通道
func Unsubscribe(ch chan string) {
	subscribersMu.Lock()
	if _, ok := subscribers[ch]; ok {
		delete(subscribers, ch)
		close(ch)
	}
	subscribersMu.Unlock()
}

// broadcast 向所有订阅者发送日志，并维护缓冲区
func broadcast(logMsg string) {

	subscribersMu.Lock()
	// 维护固定大小的缓冲区
	logBuffer = append(logBuffer, logMsg)
	if len(logBuffer) > bufferSize {
		logBuffer = logBuffer[len(logBuffer)-bufferSize:]
	}

	for ch := range subscribers {
		select {
		case ch <- logMsg:
		default:
			// 如果订阅者的缓冲已满，则丢弃该条消息，避免阻塞
		}
	}
	subscribersMu.Unlock()
}

// 自定义 writer，给 zap 使用
type ChannelWriter struct{}

func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	logMsg := string(p)
	broadcast(logMsg)
	return len(p), nil
}
