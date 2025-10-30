package composeapi

import "github.com/docker/compose/v2/pkg/api"

var _ api.LogConsumer = (*LogCompose)(nil)

type LogCompose struct {
	ch chan<- StreamMessage
}

func (l *LogCompose) Log(containerName, message string) {
	l.ch <- StreamMessage{
		Type:    MessageTypeLog,
		Content: message,
	}
}

func (l *LogCompose) Err(containerName, message string) {
	l.ch <- StreamMessage{
		Type:    MessageTypeError,
		Content: message,
	}
}

func (l *LogCompose) Status(container, msg string) {
	l.ch <- StreamMessage{
		Type:    MessageTypeStatus,
		Content: msg,
	}
}
