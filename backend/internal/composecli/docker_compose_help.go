package composecli

import (
	"bufio"
	"context"
	"io"
	"os/exec"

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
	Reader io.ReadCloser // 可以从中读取命令输出的流
	Error  error         // 启动命令时的错误
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

	result := &ExecDockerComposeStreamResult{}

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
		}()

		// 创建通道来协调stdout和stderr的处理
		done := make(chan error, 2)

		// 处理stdout
		go func() {
			defer func() { done <- nil }()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				select {
				case <-cmdCtx.Done():
					return // 上下文已取消
				default:
					line := scanner.Text() + "\n"
					if _, err := writer.Write([]byte(line)); err != nil {
						return // 管道已关闭，reader端断开连接
					}
				}
			}
		}()

		// 处理stderr
		go func() {
			defer func() { done <- nil }()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				select {
				case <-cmdCtx.Done():
					return // 上下文已取消
				default:
					line := scanner.Text() + "\n"
					if _, err := writer.Write([]byte(line)); err != nil {
						return // 管道已关闭，reader端断开连接
					}
				}
			}
		}()

		// 等待两个流处理完成或上下文取消
		go func() {
			<-done
			<-done
		}()

		// 等待命令完成或上下文取消
		<-cmdCtx.Done()
		// 上下文取消，尝试终止命令
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	return result
}
