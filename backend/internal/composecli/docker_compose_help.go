package composecli

import (
	"context"
	"io"
	"os/exec"
	"strings"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
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
	// 先取消上下文，停止命令执行
	if cr.cancel != nil {
		cr.cancel()
	}
	// 然后关闭reader
	return cr.reader.Close()
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

// ExecuteDockerComposeCommandStream 流式执行Docker Compose命令，返回可读取的输出流
//
// 使用方式：
//
//	result := ExecuteDockerComposeCommandStream(ctx, options)
//	if result.Error != nil { ... }
//	defer result.Reader.Close()
//
//	// 方式1：直接复制到WebSocket或其他Writer
//	io.Copy(websocketWriter, result.Reader)
//
//	// 方式2：逐块读取并处理
//	buffer := make([]byte, 1024)
//	for {
//	  n, err := result.Reader.Read(buffer)
//	  if n > 0 { /* 处理数据 */ }
//	  if err == io.EOF { break }
//	}
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

		// 创建通道来协调stdout和stderr的处理
		done := make(chan error, 2)

		// 处理stdout - 使用字节流复制保留原始格式
		go func() {
			defer func() { done <- nil }()
			buffer := make([]byte, 1024)
			for {
				select {
				case <-cmdCtx.Done():
					return // 上下文已取消
				default:
					n, err := stdout.Read(buffer)
					if n > 0 {
						// 直接写入，保留所有控制字符（\r, \n, ANSI等）
						if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
							return // 管道已关闭，reader端断开连接
						}
					}
					if err != nil {
						if err != io.EOF {
							logger.Logger.Error("读取stdout失败", zap.Error(err))
						}
						return
					}
				}
			}
		}()

		// 处理stderr - 使用字节流复制保留原始格式
		go func() {
			defer func() { done <- nil }()
			buffer := make([]byte, 1024)
			for {
				select {
				case <-cmdCtx.Done():
					return // 上下文已取消
				default:
					n, err := stderr.Read(buffer)
					if n > 0 {
						// 直接写入，保留所有控制字符（\r, \n, ANSI等）
						if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
							return // 管道已关闭，reader端断开连接
						}
					}
					if err != nil {
						if err != io.EOF {
							logger.Logger.Error("读取stderr失败", zap.Error(err))
						}
						return
					}
				}
			}
		}()

		// 等待两个流处理完成
		<-done
		<-done

		// 等待命令完成并获取退出码
		waitErr := cmd.Wait()
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
						zap.String("operation", options.OperationName),
						zap.String("reason", errMsg))
				} else {
					// 真正的命令执行失败
					logger.Logger.Warn("命令执行失败",
						zap.String("operation", options.OperationName),
						zap.Int("exitCode", exitCode),
						zap.Error(waitErr))
				}
			} else {
				// 其他错误，设置退出码为 -1
				exitCode = -1
				// 同样检查是否是 context 取消
				if ctx.Err() == context.Canceled {
					logger.Logger.Info("命令被取消",
						zap.String("operation", options.OperationName),
						zap.Error(waitErr))
				} else {
					logger.Logger.Error("命令等待失败",
						zap.String("operation", options.OperationName),
						zap.Error(waitErr))
				}
			}
		} else {
			logger.Logger.Info("命令执行成功",
				zap.String("operation", options.OperationName))
		}

		// 发送退出码
		result.ExitCode <- exitCode
	}()

	return result
}
