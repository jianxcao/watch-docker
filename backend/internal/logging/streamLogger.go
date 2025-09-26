package logger

import (
	"encoding/json"
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

	// 立即发送历史日志数组（如果有的话）
	if len(snapshot) > 0 {
		go func() {
			// 构造包含所有历史日志的数组消息
			historyMsg := createArrayMessage(snapshot)
			ch <- historyMsg
		}()
	}

	return ch
}

// createArrayMessage 创建日志数组消息
func createArrayMessage(logs []string) string {
	// 将字符串日志转换为 json.RawMessage 数组
	logArray := make([]json.RawMessage, len(logs))
	for i, log := range logs {
		logArray[i] = json.RawMessage(log)
	}

	// 使用 json.Marshal 序列化数组
	result, err := json.Marshal(logArray)
	if err != nil {
		// 如果序列化失败，返回空数组
		return "[]"
	}
	return string(result)
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

	// 创建包含单个日志的数组消息
	logArray := []json.RawMessage{json.RawMessage(logMsg)}
	arrayMsg, err := json.Marshal(logArray)
	if err != nil {
		// 如果序列化失败，使用空数组
		arrayMsg = []byte("[]")
	}

	for ch := range subscribers {
		select {
		case ch <- string(arrayMsg):
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
