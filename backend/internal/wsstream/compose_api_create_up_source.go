package wsstream

import (
	"context"
	"fmt"

	"github.com/jianxcao/watch-docker/backend/internal/composeapi"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposeCreateUpApiSource 实现了 Compose 创建并启动的流式数据源
// 在 Start 方法中完全控制流的内容：前置检查 -> 创建项目 -> 启动项目
type ComposeCreateUpApiSource struct {
	projectName string
	yamlContent string
	force       bool
	composeDir  string
	key         string

	// 依赖项
	composeClient interface {
		SaveNewProject(ctx context.Context, name, yamlContent string, force bool) (string, error)
		CreateProject(ctx context.Context, composeFile string, isRunning bool, isBuild bool) (<-chan composeapi.StreamMessage, error)
	}

	// 用于提前返回错误或完成消息的函数
	onComplete func(composeDir string)
}

// ComposeCreateUpApiSourceOptions 选项
type ComposeCreateUpApiSourceOptions struct {
	ProjectName   string
	YamlContent   string
	Force         bool
	ComposeDir    string
	ComposeClient interface {
		SaveNewProject(ctx context.Context, name, yamlContent string, force bool) (string, error)
		CreateProject(ctx context.Context, composeFile string, isRunning bool, isBuild bool) (<-chan composeapi.StreamMessage, error)
	}
	OnComplete func(composeDir string) // 完成回调
}

// NewComposeCreateUpApiSource 创建新的数据源
func NewComposeCreateUpApiSource(opts ComposeCreateUpApiSourceOptions) *ComposeCreateUpApiSource {
	return &ComposeCreateUpApiSource{
		projectName:   opts.ProjectName,
		yamlContent:   opts.YamlContent,
		force:         opts.Force,
		composeDir:    opts.ComposeDir,
		key:           fmt.Sprintf("compose-up-%s", opts.ProjectName),
		composeClient: opts.ComposeClient,
		onComplete:    opts.OnComplete,
	}
}

// Start 启动流，完全控制流的内容
func (s *ComposeCreateUpApiSource) Start(ctx context.Context) (StreamReader[string], error) {
	logger.Logger.Info("启动 Compose Create and Up 流",
		zap.String("projectName", s.projectName),
		zap.String("composeDir", s.composeDir))

	// 创建输出 channel
	outputChan := make(chan string, 100)

	// 在 goroutine 中控制整个流程
	go s.processFlow(ctx, outputChan)

	// 使用 ChannelStreamReader 读取 channel
	return NewChannelStreamReader(outputChan), nil
}

// processFlow 处理完整的创建和启动流程
func (s *ComposeCreateUpApiSource) processFlow(ctx context.Context, outputChan chan<- string) {
	defer close(outputChan)

	// ANSI 颜色代码
	const (
		colorReset   = "\x1b[0m"
		colorInfo    = "\x1b[36m" // 青色 (Cyan)
		colorSuccess = "\x1b[32m" // 绿色 (Green)
		colorError   = "\x1b[31m" // 红色 (Red)
		colorWarn    = "\x1b[33m" // 黄色 (Yellow)
	)

	// 辅助函数：写入带颜色的消息
	writeInfo := func(message string) {
		select {
		case outputChan <- colorInfo + message + colorReset:
		case <-ctx.Done():
		}
	}

	writeSuccess := func(message string) {
		select {
		case outputChan <- colorSuccess + message + colorReset:
		case <-ctx.Done():
		}
	}

	writeError := func(message string) {
		select {
		case outputChan <- colorError + message + colorReset:
		case <-ctx.Done():
		}
	}

	// 写入状态消息（JSON格式，用于前端判断结果）
	writeStatus := func(status, message string) {
		statusMsg := fmt.Sprintf(`{"status":"%s","message":"%s"}`, status, escapeJSON(message))
		// 在JSON前加一个特殊标记，让前端知道这是状态消息
		select {
		case outputChan <- "\x00JSON:" + statusMsg:
		case <-ctx.Done():
		}
	}

	// 1. 如果有 yaml 内容，先创建项目文件
	var composeFile string
	if s.yamlContent != "" {
		// 发送信息：正在创建项目
		writeInfo(fmt.Sprintf("正在创建或更新项目 %s...\r\n", s.projectName))

		// 创建项目
		var err error
		composeFile, err = s.composeClient.SaveNewProject(ctx, s.projectName, s.yamlContent, s.force)
		if err != nil {
			// 创建失败，写入错误并结束
			errMsg := fmt.Sprintf("创建或更新项目文件失败: %s\r\n", err.Error())
			writeError(errMsg)
			logger.Logger.Error("创建项目失败", zap.Error(err))
			// 发送失败状态
			writeStatus("error", "创建项目失败: "+err.Error())
			return // 直接结束，不执行后续命令
		}

		// 创建成功
		successMsg := fmt.Sprintf("✓ 项目文件创建或更新成功: %s\r\n", composeFile)
		writeSuccess(successMsg)
	} else {
		// 如果没有 yaml 内容，需要构造 compose 文件路径
		composeFile = fmt.Sprintf("%s/docker-compose.yaml", s.composeDir)
	}

	// 2. 发送信息：正在启动项目
	writeInfo("正在启动项目...\r\n")

	// 3. 使用 composeapi 创建并启动项目
	ch, err := s.composeClient.CreateProject(ctx, composeFile, true, false)
	if err != nil {
		errMsg := fmt.Sprintf("启动项目失败: %s\r\n", err.Error())
		writeError(errMsg)
		logger.Logger.Error("启动命令失败", zap.Error(err))
		writeStatus("error", "启动项目失败: "+err.Error())
		return
	}

	// 4. 读取流式输出
	var hasError bool
	for msg := range ch {
		switch msg.Type {
		case composeapi.MessageTypeLog:
			// 写入日志消息
			if msg.Content != "" {
				writeInfo(msg.Content)
			}
		case composeapi.MessageTypeError:
			// 写入错误消息
			errMsg := fmt.Sprintf("错误: %s\r\n", msg.Error.Error())
			writeError(errMsg)
			hasError = true
		case composeapi.MessageTypeComplete:
			// 完成消息
			if msg.Content != "" {
				writeSuccess(msg.Content + "\r\n")
			}
		}
	}

	if hasError {
		writeError("\r\n✗ 项目启动失败，请检查配置和日志\r\n")
		writeStatus("error", "项目启动失败")
		return
	}

	// 5. 发送完成消息
	writeSuccess(fmt.Sprintf("\r\n✓ 项目启动完成: %s\r\n", composeFile))

	// 发送成功状态（让前端知道操作成功）
	writeStatus("success", composeFile)

	// 调用完成回调
	if s.onComplete != nil {
		s.onComplete(s.composeDir)
	}
}

// Stop 停止流
func (s *ComposeCreateUpApiSource) Stop() error {
	logger.Logger.Info("停止 Compose Create and Up 流",
		zap.String("projectName", s.projectName))
	// 由于我们使用的是 channel，关闭会自动停止
	return nil
}

// GetKey 获取唯一标识
func (s *ComposeCreateUpApiSource) GetKey() string {
	return s.key
}
