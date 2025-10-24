package composecli

import (
	"context"
	"io"
	"os/exec"
	"strings"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type ExecDockerComposeOptions struct {
	ExecPath      string
	Args          []string // docker compose 命令参数（不包括 "docker"）
	OperationName string   // 操作名称，用于日志
	NeedOutput    bool     // 是否需要捕获输出
}

type ExecDockerComposeResult struct {
	Output []byte
	Error  error
}

// ExecDockerComposeStreamOptions 流式执行Docker Compose命令的选项
type ExecDockerComposeStreamOptions struct {
	ExecPath      string
	Args          []string // docker compose 命令参数（不包括 "docker"）
	OperationName string   // 操作名称，用于日志
}

// ExecDockerComposeStreamResult 流式执行Docker Compose命令的结果
type ExecDockerComposeStreamResult struct {
	Reader   io.ReadCloser // 可以从中读取命令输出的流
	Error    error         // 启动命令时的错误
	ExitCode chan int      // 命令退出码（异步获取，命令执行完成后会发送）
}

// cmdReader 包装了reader和cmd，确保关闭reader时也能停止cmd
type cmdReader struct {
	reader io.ReadCloser
	cancel context.CancelFunc
}

func (cr *cmdReader) Read(p []byte) (n int, err error) {
	return cr.reader.Read(p)
}

func (cr *cmdReader) Close() error {
	logger.Logger.Info("cmdReader.Close() called - cancelling context and closing reader")

	// 1. 取消上下文，这会：
	//    - kill docker compose 进程
	//    - 让 stdout/stderr goroutines 通过 cmdCtx.Done() 检测到取消
	if cr.cancel != nil {
		cr.cancel()
	}

	// 2. 关闭 reader
	return cr.reader.Close()
}

// copyStreamWithCancel 可取消的流复制函数
// 从 src 读取数据并写入 dst，支持通过 context 取消
func copyStreamWithCancel(ctx context.Context, dst io.Writer, src io.Reader, streamName string) error {
	// 创建 channel 来传递读取结果
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)

	// 启动读取 goroutine
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := src.Read(buffer)
			// 复制数据到新的 slice，避免竞态
			var data []byte
			if n > 0 {
				data = make([]byte, n)
				copy(data, buffer[:n])
			}

			select {
			case readCh <- readResult{data, err}:
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 主循环：等待读取结果或 context 取消
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Debug(streamName+" goroutine cancelled by context", zap.String("stream", streamName))
			return ctx.Err()
		case result := <-readCh:
			if len(result.data) > 0 {
				if _, writeErr := dst.Write(result.data); writeErr != nil {
					logger.Logger.Info(streamName+" write to pipe failed",
						zap.String("stream", streamName),
						zap.Error(writeErr))
					return writeErr
				}
			}
			if result.err != nil {
				if result.err != io.EOF {
					logger.Logger.Error("读取"+streamName+"失败",
						zap.String("stream", streamName),
						zap.Error(result.err))
					return result.err
				}
				// EOF 是正常结束
				logger.Logger.Debug(streamName+" reached EOF", zap.String("stream", streamName))
				return nil
			}
		}
	}
}

// parseExitCode 解析命令退出码并记录日志
func parseExitCode(waitErr error, ctx context.Context, operationName string) int {
	exitCode := 0

	if waitErr != nil {
		// 检查是否是退出码错误
		if exitError, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()

			// 检查是否是因为 context 取消或信号导致的终止（这是正常的取消操作）
			errMsg := waitErr.Error()
			if strings.Contains(errMsg, "signal: killed") ||
				strings.Contains(errMsg, "signal: terminated") ||
				strings.Contains(errMsg, "signal: interrupt") {
				logger.Logger.Info("命令被取消",
					zap.String("operation", operationName),
					zap.String("reason", errMsg))
			} else {
				// 真正的命令执行失败
				logger.Logger.Warn("命令执行失败",
					zap.String("operation", operationName),
					zap.Int("exitCode", exitCode),
					zap.Error(waitErr))
			}
		} else {
			// 其他错误，设置退出码为 -1
			exitCode = -1
			// 同样检查是否是 context 取消
			if ctx.Err() == context.Canceled {
				logger.Logger.Info("命令被取消",
					zap.String("operation", operationName),
					zap.Error(waitErr))
			} else {
				logger.Logger.Error("命令等待失败",
					zap.String("operation", operationName),
					zap.Error(waitErr))
			}
		}
	} else {
		logger.Logger.Info("命令执行成功",
			zap.String("operation", operationName))
	}

	return exitCode
}

func ExecuteDockerComposeCommand(ctx context.Context, options ExecDockerComposeOptions) *ExecDockerComposeResult {
	fullArgs := append([]string{"compose"}, options.Args...)
	logger.Logger.Info("ExecuteDockerComposeCommand", zap.String("execPath", options.ExecPath), zap.Strings("args", fullArgs))
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = options.ExecPath
	result := &ExecDockerComposeResult{}
	if options.NeedOutput {
		result.Output, result.Error = cmd.CombinedOutput()
	} else {
		result.Error = cmd.Run()
	}
	return result
}

func ExecuteDockerComposeCommandStream(ctx context.Context, options ExecDockerComposeStreamOptions) *ExecDockerComposeStreamResult {
	// 创建可取消的上下文，用于控制命令执行
	cmdCtx, cancel := context.WithCancel(ctx)

	fullArgs := append([]string{"compose"}, options.Args...)
	cmd := exec.CommandContext(cmdCtx, "docker", fullArgs...)
	cmd.Dir = options.ExecPath
	logger.Logger.Info("ExecuteDockerComposeCommandStream", zap.String("args", strings.Join(fullArgs, " ")))

	result := &ExecDockerComposeStreamResult{
		ExitCode: make(chan int, 1), // 缓冲通道，确保不会阻塞
	}

	// 创建管道用于数据传输
	reader, writer := io.Pipe()

	// 使用包装的reader，确保关闭时能停止cmd
	result.Reader = &cmdReader{
		reader: reader,
		cancel: cancel,
	}

	// 获取命令的输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Error = err
		cancel()
		writer.Close()
		return result
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		result.Error = err
		cancel()
		writer.Close()
		return result
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		result.Error = err
		cancel()
		writer.Close()
		return result
	}

	// 在goroutine中处理命令输出并写入管道
	go func() {
		defer func() {
			writer.Close()
			// 确保命令被清理
			cancel()
			// 关闭退出码通道
			close(result.ExitCode)
		}()

		// 使用 errgroup 管理 stdout 和 stderr 的并发复制
		eg, egCtx := errgroup.WithContext(cmdCtx)

		// 并发复制 stdout
		eg.Go(func() error {
			defer logger.Logger.Debug("stdout goroutine exiting")
			return copyStreamWithCancel(egCtx, writer, stdout, "stdout")
		})

		// 并发复制 stderr
		eg.Go(func() error {
			defer logger.Logger.Debug("stderr goroutine exiting")
			return copyStreamWithCancel(egCtx, writer, stderr, "stderr")
		})

		// 等待两个流都完成（忽略错误，因为取消是正常的）
		_ = eg.Wait()

		// 等待命令完成并解析退出码
		exitCode := parseExitCode(cmd.Wait(), ctx, options.OperationName)

		// 发送退出码
		result.ExitCode <- exitCode
	}()

	return result
}
