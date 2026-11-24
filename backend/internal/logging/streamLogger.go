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

// Subscribe 创建一个新的日志订阅通道，并在持锁期间同步写入历史缓冲区日志，
// 避免异步 goroutine 向已关闭的 channel 发送导致 panic，以及历史与实时日志乱序。
func Subscribe() chan string {
	ch := make(chan string, bufferSize+100)

	subscribersMu.Lock()
	if len(logBuffer) > 0 {
		historyMsg := createArrayMessage(logBuffer)
		select {
		case ch <- historyMsg:
		default:
		}
	}
	subscribers[ch] = struct{}{}
	subscribersMu.Unlock()

	return ch
}

// createArrayMessage 创建日志数组消息
func createArrayMessage(logs []string) string {
	logArray := make([]json.RawMessage, len(logs))
	for i, log := range logs {
		logArray[i] = json.RawMessage(log)
	}

	result, err := json.Marshal(logArray)
	if err != nil {
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
	logBuffer = append(logBuffer, logMsg)
	if len(logBuffer) > bufferSize {
		logBuffer = logBuffer[len(logBuffer)-bufferSize:]
	}

	logArray := []json.RawMessage{json.RawMessage(logMsg)}
	arrayMsg, err := json.Marshal(logArray)
	if err != nil {
		arrayMsg = []byte("[]")
	}

	msgStr := string(arrayMsg)
	for ch := range subscribers {
		select {
		case ch <- msgStr:
		default:
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
