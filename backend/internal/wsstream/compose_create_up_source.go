package wsstream

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposeCreateUpSource 实现了 Compose 创建并启动的流式数据源
// 在 Start 方法中完全控制流的内容：前置检查 -> 创建项目 -> 启动项目
type ComposeCreateUpSource struct {
	projectName string
	yamlContent string
	composeFile string
	force       bool
	composeDir  string
	key         string

	// 依赖项
	composeClient interface {
		SaveProjectPlanForDeploy(ctx context.Context, name, composeFile, yamlContent string, force bool) (composecli.DeployProjectPlan, error)
		ResolveDeployPlan(ctx context.Context, name, composeFile string) (composecli.DeployProjectPlan, error)
	}
	streamExecutor func(ctx context.Context, options composecli.ExecDockerComposeStreamOptions) *composecli.ExecDockerComposeStreamResult

	// 用于提前返回错误或完成消息的函数
	onComplete func(composeDir string)
}

// ComposeCreateUpSourceOptions 选项
type ComposeCreateUpSourceOptions struct {
	ProjectName   string
	YamlContent   string
	ComposeFile   string
	Force         bool
	ComposeDir    string
	ComposeClient interface {
		SaveProjectPlanForDeploy(ctx context.Context, name, composeFile, yamlContent string, force bool) (composecli.DeployProjectPlan, error)
		ResolveDeployPlan(ctx context.Context, name, composeFile string) (composecli.DeployProjectPlan, error)
	}
	StreamExecutor func(ctx context.Context, options composecli.ExecDockerComposeStreamOptions) *composecli.ExecDockerComposeStreamResult
	OnComplete     func(composeDir string) // 完成回调
}

// NewComposeCreateUpSource 创建新的数据源
func NewComposeCreateUpSource(opts ComposeCreateUpSourceOptions) *ComposeCreateUpSource {
	executor := opts.StreamExecutor
	if executor == nil {
		executor = composecli.ExecuteDockerComposeCommandStream
	}
	return &ComposeCreateUpSource{
		projectName:    opts.ProjectName,
		yamlContent:    opts.YamlContent,
		composeFile:    opts.ComposeFile,
		force:          opts.Force,
		composeDir:     opts.ComposeDir,
		key:            fmt.Sprintf("compose-up-%s", opts.ProjectName),
		composeClient:  opts.ComposeClient,
		streamExecutor: executor,
		onComplete:     opts.OnComplete,
	}
}

// Start 启动流，完全控制流的内容
func (s *ComposeCreateUpSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动 Compose Create and Up 流",
		zap.String("projectName", s.projectName),
		zap.String("composeDir", s.composeDir))

	// 创建管道，我们完全控制写入内容
	reader, writer := io.Pipe()

	// 在 goroutine 中控制整个流程
	go s.processFlow(ctx, writer)

	// 使用 ByteStreamReader 直接流式传输
	return NewByteStreamReader(reader), nil
}

// processFlow 处理完整的创建和启动流程
func (s *ComposeCreateUpSource) processFlow(ctx context.Context, writer *io.PipeWriter) {
	defer writer.Close()

	// ANSI 颜色代码
	const (
		colorReset   = "\x1b[0m"
		colorInfo    = "\x1b[36m" // 青色 (Cyan)
		colorSuccess = "\x1b[32m" // 绿色 (Green)
		colorError   = "\x1b[31m" // 红色 (Red)
		colorWarn    = "\x1b[33m" // 黄色 (Yellow)
	)

	// 辅助函数：写入带颜色的消息
	writeInfo := func(message string) error {
		_, err := writer.Write([]byte(colorInfo + message + colorReset))
		return err
	}

	writeSuccess := func(message string) error {
		_, err := writer.Write([]byte(colorSuccess + message + colorReset))
		return err
	}

	writeError := func(message string) error {
		_, err := writer.Write([]byte(colorError + message + colorReset))
		return err
	}

	writeRaw := func(data []byte) error {
		_, err := writer.Write(data)
		return err
	}

	// 写入状态消息（JSON格式，用于前端判断结果）
	writeStatus := func(status, message string) error {
		statusMsg := fmt.Sprintf(`{"status":"%s","message":"%s"}`, status, escapeJSON(message))
		// 在JSON前加一个特殊标记，让前端知道这是状态消息
		_, err := writer.Write([]byte("\x00JSON:" + statusMsg))
		return err
	}

	var plan composecli.DeployProjectPlan
	execDir := s.composeDir

	// 1. 如果有 yaml 内容，先保存用户编辑的文件并解析完整部署方案（含运行时 -p 项目名与有序 -f 文件列表）；无 yaml 时同样校验并解析
	if s.yamlContent != "" {
		// 发送信息：正在创建项目
		if err := writeInfo(fmt.Sprintf("正在创建或更新项目 %s...\r\n", s.projectName)); err != nil {
			logger.Logger.Error("写入创建提示失败", zap.Error(err))
			return
		}

		var err error
		plan, err = s.composeClient.SaveProjectPlanForDeploy(ctx, s.projectName, s.composeFile, s.yamlContent, s.force)
		if err != nil {
			// 创建失败，写入错误并结束
			errMsg := fmt.Sprintf("创建或更新项目文件失败: %s\r\n", err.Error())
			if err := writeError(errMsg); err != nil {
				logger.Logger.Error("写入错误消息失败", zap.Error(err))
			}
			logger.Logger.Error("创建项目失败", zap.Error(err))
			// 发送失败状态
			_ = writeStatus("error", "创建项目失败: "+err.Error())
			return // 直接结束，不执行后续命令
		}

		if plan.SavedFile != "" {
			execDir = filepath.Dir(plan.SavedFile)
		} else if plan.ProjectDir != "" {
			execDir = plan.ProjectDir
		}

		// 创建成功
		successMsg := fmt.Sprintf("✓ 项目文件创建或更新成功: %s\r\n", plan.SavedFile)
		if err := writeSuccess(successMsg); err != nil {
			logger.Logger.Error("写入成功消息失败", zap.Error(err))
			return
		}
	} else if s.composeClient != nil {
		var err error
		plan, err = s.composeClient.ResolveDeployPlan(ctx, s.projectName, s.composeFile)
		if err != nil {
			errMsg := fmt.Sprintf("校验 Compose 文件列表失败: %s\r\n", err.Error())
			_ = writeError(errMsg)
			_ = writeStatus("error", "创建项目失败: "+err.Error())
			return
		}
		if plan.ProjectDir != "" {
			execDir = plan.ProjectDir
		}
	}

	// 2. 发送信息：正在启动项目
	if err := writeInfo("正在启动项目...\r\n"); err != nil {
		logger.Logger.Error("写入启动提示失败", zap.Error(err))
		return
	}

	// 3. 执行 docker compose up 命令（up 与后续 ps 共用同一组 -p <runtimeName> 与有序 -f 参数）
	prefixArgs := plan.ComposeArgsPrefix()
	upArgs := make([]string, 0, len(prefixArgs)+6)
	upArgs = append(upArgs, prefixArgs...)
	upArgs = append(upArgs, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate")
	result := s.streamExecutor(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      execDir,
		Args:          upArgs,
		OperationName: "compose up",
	})

	if result.Error != nil {
		errMsg := fmt.Sprintf("启动项目命令执行失败: %s", result.Error.Error())
		if err := writeError(errMsg + "\r\n"); err != nil {
			logger.Logger.Error("写入错误消息失败", zap.Error(err))
		}
		_ = writeStatus("error", errMsg)
		logger.Logger.Error("启动命令失败", zap.Error(result.Error))
		return
	}

	defer result.Reader.Close()

	// 4. 读取 docker compose up 的输出并直接转发到我们的管道
	buffer := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Info("上下文取消，停止读取输出")
			writeError("\r\n操作超时或被取消\r\n")
			return
		default:
			n, err := result.Reader.Read(buffer)
			if n > 0 {
				// 直接转发原始输出（包含 ANSI 颜色）
				if err := writeRaw(buffer[:n]); err != nil {
					logger.Logger.Error("写入日志消息失败", zap.Error(err))
					return
				}
			}

			if err != nil {
				if err == io.EOF {
					logger.Logger.Info("compose up 命令输出读取完成")
					goto afterUpCommand
				}
				logger.Logger.Error("读取 compose up 输出失败", zap.Error(err))
				errMsg := fmt.Sprintf("读取输出失败: %s", err.Error())
				_ = writeError(errMsg + "\r\n")
				_ = writeStatus("error", errMsg)
				return
			}
		}
	}

afterUpCommand:
	// 5. 等待命令退出码
	var hasError bool
	select {
	case exitCode, ok := <-result.ExitCode:
		if !ok {
			logger.Logger.Error("退出码通道异常关闭")
			hasError = true
		} else {
			logger.Logger.Info("收到 compose up 退出码", zap.Int("exitCode", exitCode))
			if exitCode != 0 {
				hasError = true
				logger.Logger.Error("compose up 执行失败", zap.Int("exitCode", exitCode))
			}
		}
	case <-ctx.Done():
		logger.Logger.Warn("等待退出码时上下文取消")
		writeError("\r\n操作被取消\r\n")
		return
	}

	if hasError {
		writeError("\r\n✗ 项目启动失败，请检查配置和日志\r\n")
		// 发送失败状态
		writeStatus("error", "项目启动失败")
		return
	}

	// 6. 获取项目状态（与 up 共用同一组 -p 与有序 -f 参数）
	writeInfo("\r\n正在获取项目状态...\r\n")

	psArgs := make([]string, 0, len(prefixArgs)+1)
	psArgs = append(psArgs, prefixArgs...)
	psArgs = append(psArgs, "ps")
	statusResult := s.streamExecutor(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      execDir,
		Args:          psArgs,
		OperationName: "compose ps",
	})

	if statusResult.Error == nil && statusResult.Reader != nil {
		defer statusResult.Reader.Close()
		statusBuffer := make([]byte, 4096)
		for {
			n, err := statusResult.Reader.Read(statusBuffer)
			if n > 0 {
				writeRaw(statusBuffer[:n])
			}
			if err != nil {
				break
			}
		}
	}

	// 7. 发送完成消息
	writeSuccess(fmt.Sprintf("\r\n✓ 项目启动完成: %s\r\n", execDir))

	// 发送成功状态（让前端知道操作成功）
	writeStatus("success", execDir)

	// 调用完成回调
	if s.onComplete != nil {
		s.onComplete(execDir)
	}
}

// escapeJSON 转义 JSON 字符串中的特殊字符
func escapeJSON(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		case '\n':
			result += "\\n"
		case '\r':
			result += "\\r"
		case '\t':
			result += "\\t"
		default:
			result += string(c)
		}
	}
	return result
}

// Stop 停止流
func (s *ComposeCreateUpSource) Stop() error {
	logger.Logger.Info("停止 Compose Create and Up 流",
		zap.String("projectName", s.projectName))
	// 由于我们使用的是 pipe，关闭 reader 会触发 writer 停止
	return nil
}

// GetKey 获取唯一标识
func (s *ComposeCreateUpSource) GetKey() string {
	return s.key
}
