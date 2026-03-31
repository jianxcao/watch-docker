package wsstream

import (
	"context"
	"fmt"
	"io"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposeUpExistingSource 对已有 Compose 项目执行 docker compose up -d 并流式输出
type ComposeUpExistingSource struct {
	projectPath string
	projectName string
	key         string
}

func NewComposeUpExistingSource(projectPath, projectName string) *ComposeUpExistingSource {
	return &ComposeUpExistingSource{
		projectPath: projectPath,
		projectName: projectName,
		key:         fmt.Sprintf("compose-up-existing-%s", projectName),
	}
}

func (s *ComposeUpExistingSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动 Compose Up 流（已有项目）",
		zap.String("projectPath", s.projectPath),
		zap.String("projectName", s.projectName))

	reader, writer := io.Pipe()
	go s.processFlow(ctx, writer)
	return NewByteStreamReader(reader), nil
}

func (s *ComposeUpExistingSource) processFlow(ctx context.Context, writer *io.PipeWriter) {
	defer writer.Close()

	const (
		colorReset   = "\x1b[0m"
		colorInfo    = "\x1b[36m"
		colorSuccess = "\x1b[32m"
		colorError   = "\x1b[31m"
	)

	writeInfo := func(msg string) {
		writer.Write([]byte(colorInfo + msg + colorReset))
	}
	writeSuccess := func(msg string) {
		writer.Write([]byte(colorSuccess + msg + colorReset))
	}
	writeError := func(msg string) {
		writer.Write([]byte(colorError + msg + colorReset))
	}
	writeStatus := func(status, message string) {
		statusMsg := fmt.Sprintf(`{"status":"%s","message":"%s"}`, status, escapeJSON(message))
		writer.Write([]byte("\x00JSON:" + statusMsg))
	}

	writeInfo(fmt.Sprintf("正在创建/重建项目 %s...\r\n", s.projectName))

	result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      s.projectPath,
		Args:          []string{"--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"},
		OperationName: "compose up (existing)",
	})

	if result.Error != nil {
		writeError(fmt.Sprintf("执行失败: %s\r\n", result.Error.Error()))
		writeStatus("error", result.Error.Error())
		return
	}
	defer result.Reader.Close()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			writeError("\r\n操作被取消\r\n")
			return
		default:
			n, err := result.Reader.Read(buf)
			if n > 0 {
				writer.Write(buf[:n])
			}
			if err != nil {
				if err == io.EOF {
					goto afterUp
				}
				writeError(fmt.Sprintf("读取输出失败: %s\r\n", err.Error()))
				return
			}
		}
	}

afterUp:
	var hasError bool
	select {
	case exitCode, ok := <-result.ExitCode:
		if !ok || exitCode != 0 {
			hasError = true
		}
	case <-ctx.Done():
		writeError("\r\n操作被取消\r\n")
		return
	}

	if hasError {
		writeError("\r\n✗ 项目创建/重建失败\r\n")
		writeStatus("error", "项目创建/重建失败")
		return
	}

	writeInfo("\r\n正在获取项目状态...\r\n")
	statusResult := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      s.projectPath,
		Args:          []string{"ps"},
		OperationName: "compose ps",
	})
	if statusResult.Error == nil && statusResult.Reader != nil {
		defer statusResult.Reader.Close()
		statusBuf := make([]byte, 4096)
		for {
			n, err := statusResult.Reader.Read(statusBuf)
			if n > 0 {
				writer.Write(statusBuf[:n])
			}
			if err != nil {
				break
			}
		}
	}

	writeSuccess(fmt.Sprintf("\r\n✓ 项目创建/重建完成: %s\r\n", s.projectName))
	writeStatus("success", s.projectName)
}

func (s *ComposeUpExistingSource) Stop() error {
	return nil
}

func (s *ComposeUpExistingSource) GetKey() string {
	return s.key
}
