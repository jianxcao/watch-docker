package composecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteDockerComposeCommand(t *testing.T) {
	tests := []struct {
		name           string
		options        ExecDockerComposeOptions
		expectError    bool
		validateResult func(t *testing.T, result *ExecDockerComposeResult)
	}{
		{
			name: "执行简单命令不需要输出",
			options: ExecDockerComposeOptions{
				ExecPath:      ".",
				Args:          []string{"version"},
				OperationName: "test version",
				NeedOutput:    false,
			},
			expectError: false,
			validateResult: func(t *testing.T, result *ExecDockerComposeResult) {
				if result.Output != nil {
					t.Error("不需要输出时Output应该为nil")
				}
			},
		},
		{
			name: "执行命令需要输出",
			options: ExecDockerComposeOptions{
				ExecPath:      ".",
				Args:          []string{"version"},
				OperationName: "test version with output",
				NeedOutput:    true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result *ExecDockerComposeResult) {
				s := string(result.Output)
				fmt.Println("执行结果", s)
				if result.Output == nil {
					t.Error("需要输出时Output不应该为nil")
				}
			},
		},
		{
			name: "执行不存在的命令",
			options: ExecDockerComposeOptions{
				ExecPath:      ".",
				Args:          []string{"nonexistent-command"},
				OperationName: "test nonexistent",
				NeedOutput:    true,
			},
			expectError: true,
			validateResult: func(t *testing.T, result *ExecDockerComposeResult) {
				// 错误情况下可能有输出也可能没有，这取决于具体的错误类型
			},
		},
		{
			name: "使用不存在的路径",
			options: ExecDockerComposeOptions{
				ExecPath:      "/nonexistent/path",
				Args:          []string{"version"},
				OperationName: "test nonexistent path",
				NeedOutput:    false,
			},
			expectError: true,
			validateResult: func(t *testing.T, result *ExecDockerComposeResult) {
				// 错误情况验证
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result := ExecuteDockerComposeCommand(ctx, tt.options)

			if tt.expectError {
				if result.Error == nil {
					t.Error("期望有错误但没有返回错误")
				}
			} else {
				if result.Error != nil {
					// 注意：某些测试环境可能没有docker compose，所以这里不强制要求成功
					t.Logf("命令执行失败（可能是环境问题）: %v", result.Error)
				}
			}

			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestExecuteDockerComposeCommandStream(t *testing.T) {
	tests := []struct {
		name           string
		options        ExecDockerComposeStreamOptions
		expectError    bool
		validateResult func(t *testing.T, result *ExecDockerComposeStreamResult)
	}{
		{
			name: "流式执行简单命令",
			options: ExecDockerComposeStreamOptions{
				ExecPath:      ".",
				Args:          []string{"version"},
				OperationName: "stream test version",
			},
			expectError: false,
			validateResult: func(t *testing.T, result *ExecDockerComposeStreamResult) {
				if result.Reader == nil {
					t.Error("Reader不应该为nil")
					return
				}
				defer result.Reader.Close()

				// 读取一些数据
				buffer := make([]byte, 1024)
				_, err := result.Reader.Read(buffer)
				if err != nil && err != io.EOF {
					t.Logf("读取流数据时出现错误（可能是环境问题）: %v", err)
				}
			},
		},
		{
			name: "流式执行不存在的命令",
			options: ExecDockerComposeStreamOptions{
				ExecPath:      ".",
				Args:          []string{"nonexistent-command"},
				OperationName: "stream test nonexistent",
			},
			expectError: false, // 流式执行在启动时可能不会立即返回错误
			validateResult: func(t *testing.T, result *ExecDockerComposeStreamResult) {
				if result.Reader != nil {
					defer result.Reader.Close()

					// 尝试读取输出，可能会在读取时遇到错误
					buffer := make([]byte, 1024)
					_, err := result.Reader.Read(buffer)
					if err != nil && err != io.EOF {
						t.Logf("读取时遇到错误（这是预期的）: %v", err)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result := ExecuteDockerComposeCommandStream(ctx, tt.options)

			if tt.expectError {
				if result.Error == nil {
					t.Error("期望有错误但没有返回错误")
				}
			} else {
				if result.Error != nil {
					// 注意：某些测试环境可能没有docker compose，所以这里不强制要求成功
					t.Logf("命令启动失败（可能是环境问题）: %v", result.Error)
				}
			}

			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestExecuteDockerComposeCommandStreamTimeout(t *testing.T) {
	// 创建一个很短的超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "timeout test",
	}

	result := ExecuteDockerComposeCommandStream(ctx, options)

	// 即使上下文已经超时，函数也应该能够正常返回
	if result == nil {
		t.Error("结果不应该为nil")
		return
	}

	if result.Reader != nil {
		defer result.Reader.Close()
	}
}

func TestExecuteDockerComposeCommandStreamReadComplete(t *testing.T) {
	// 使用一个简单的echo命令来模拟输出
	// 注意：这个测试依赖于系统有echo命令
	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "complete read test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := ExecuteDockerComposeCommandStream(ctx, options)
	if result.Reader == nil {
		if result.Error != nil {
			t.Skipf("跳过测试，命令启动失败: %v", result.Error)
		}
		t.Fatal("Reader不应该为nil")
	}
	defer result.Reader.Close()

	// 读取所有输出
	var output bytes.Buffer
	done := make(chan error, 1)

	// 在goroutine中读取，避免阻塞
	go func() {
		_, err := io.Copy(&output, result.Reader)
		done <- err
	}()

	// 等待读取完成或超时
	select {
	case err := <-done:
		if err != nil {
			t.Logf("读取完整输出时出现错误（可能是环境问题）: %v", err)
		}
		t.Logf("读取到的输出长度: %d 字节", output.Len())
	case <-ctx.Done():
		t.Log("读取超时，但这可能是正常的（命令可能需要更长时间）")
	}
}

// TestExecuteDockerComposeCommandStreamMultipleReads 测试多次读取流
func TestExecuteDockerComposeCommandStreamMultipleReads(t *testing.T) {
	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "multiple reads test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := ExecuteDockerComposeCommandStream(ctx, options)
	if result.Reader == nil {
		if result.Error != nil {
			t.Skipf("跳过测试，命令启动失败: %v", result.Error)
		}
		t.Fatal("Reader不应该为nil")
	}
	defer result.Reader.Close()

	// 进行多次小块读取
	buffer := make([]byte, 64)
	totalRead := 0
	readCount := 0

	// 设置读取超时
	readTimeout := time.After(1500 * time.Millisecond)

	for readCount < 10 { // 最多读取10次
		select {
		case <-readTimeout:
			t.Log("读取超时，可能命令输出较少或执行时间较长")
			goto readComplete
		default:
			n, err := result.Reader.Read(buffer)
			if n > 0 {
				totalRead += n
				readCount++
			}
			if err == io.EOF {
				goto readComplete
			}
			if err != nil {
				t.Logf("读取时出现错误: %v", err)
				goto readComplete
			}
		}
	}

readComplete:
	t.Logf("总共读取 %d 次，%d 字节", readCount, totalRead)
}

// BenchmarkExecuteDockerComposeCommand 性能测试
func BenchmarkExecuteDockerComposeCommand(b *testing.B) {
	options := ExecDockerComposeOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "benchmark test",
		NeedOutput:    false,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := ExecuteDockerComposeCommand(ctx, options)
		if result.Error != nil {
			b.Skip("跳过基准测试，docker compose不可用")
		}
	}
}

// BenchmarkExecuteDockerComposeCommandStream 流式执行性能测试
func BenchmarkExecuteDockerComposeCommandStream(b *testing.B) {
	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "benchmark stream test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 使用带超时的上下文
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		result := ExecuteDockerComposeCommandStream(ctx, options)
		if result.Error != nil {
			cancel()
			b.Skip("跳过基准测试，docker compose不可用")
		}
		if result.Reader != nil {
			// 在另一个goroutine中读取，避免阻塞
			done := make(chan bool, 1)
			go func() {
				defer close(done)
				buffer := make([]byte, 1024)
				for {
					_, err := result.Reader.Read(buffer)
					if err != nil {
						return
					}
				}
			}()

			// 等待读取完成或超时
			select {
			case <-done:
			case <-time.After(1500 * time.Millisecond):
			}

			result.Reader.Close()
		}
		cancel()
	}
}

// TestExecDockerComposeOptions 测试选项结构体
func TestExecDockerComposeOptions(t *testing.T) {
	options := ExecDockerComposeOptions{
		ExecPath:      "/test/path",
		Args:          []string{"arg1", "arg2"},
		OperationName: "test operation",
		NeedOutput:    true,
	}

	if options.ExecPath != "/test/path" {
		t.Error("ExecPath设置不正确")
	}
	if len(options.Args) != 2 {
		t.Error("Args长度不正确")
	}
	if options.Args[0] != "arg1" || options.Args[1] != "arg2" {
		t.Error("Args内容不正确")
	}
	if options.OperationName != "test operation" {
		t.Error("OperationName设置不正确")
	}
	if !options.NeedOutput {
		t.Error("NeedOutput设置不正确")
	}
}

// TestExecDockerComposeStreamOptions 测试流式选项结构体
func TestExecDockerComposeStreamOptions(t *testing.T) {
	options := ExecDockerComposeStreamOptions{
		ExecPath:      "/test/path",
		Args:          []string{"arg1", "arg2"},
		OperationName: "test stream operation",
	}

	if options.ExecPath != "/test/path" {
		t.Error("ExecPath设置不正确")
	}
	if len(options.Args) != 2 {
		t.Error("Args长度不正确")
	}
	if options.Args[0] != "arg1" || options.Args[1] != "arg2" {
		t.Error("Args内容不正确")
	}
	if options.OperationName != "test stream operation" {
		t.Error("OperationName设置不正确")
	}
}

// TestExecuteDockerComposeCommandStreamCancel 测试上下文取消
func TestExecuteDockerComposeCommandStreamCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "cancel test",
	}

	result := ExecuteDockerComposeCommandStream(ctx, options)
	if result.Reader == nil {
		if result.Error != nil {
			t.Skipf("跳过测试，命令启动失败: %v", result.Error)
		}
		t.Fatal("Reader不应该为nil")
	}
	defer result.Reader.Close()

	// 立即取消上下文
	cancel()

	// 尝试读取，应该能够处理取消
	buffer := make([]byte, 1024)
	_, err := result.Reader.Read(buffer)
	if err != nil && err != io.EOF && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("读取时出现错误（这可能是预期的）: %v", err)
	}
}

// TestExecuteDockerComposeCommandWithWorkingDirectory 测试工作目录
func TestExecuteDockerComposeCommandWithWorkingDirectory(t *testing.T) {
	// 创建临时目录
	tempDir := os.TempDir()

	options := ExecDockerComposeOptions{
		ExecPath:      tempDir,
		Args:          []string{"version"},
		OperationName: "working directory test",
		NeedOutput:    false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := ExecuteDockerComposeCommand(ctx, options)
	// 这个测试主要验证不会因为路径问题而panic
	// 错误是可以接受的，因为临时目录可能没有compose文件

	t.Logf("使用工作目录 %s 的结果 - 错误: %v", tempDir, result.Error)
}

// TestStreamReaderCloseTerminatesCommand 测试关闭Reader时是否能终止命令
func TestStreamReaderCloseTerminatesCommand(t *testing.T) {
	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "reader close test",
	}

	ctx := context.Background()
	result := ExecuteDockerComposeCommandStream(ctx, options)

	if result.Error != nil {
		t.Skipf("跳过测试，命令启动失败: %v", result.Error)
	}

	if result.Reader == nil {
		t.Fatal("Reader不应该为nil")
	}

	// 立即关闭Reader
	start := time.Now()
	err := result.Reader.Close()
	duration := time.Since(start)

	if err != nil {
		t.Logf("关闭Reader时出现错误: %v", err)
	}

	t.Logf("关闭Reader耗时: %v", duration)

	// 验证关闭后无法再读取
	buffer := make([]byte, 1024)
	_, err = result.Reader.Read(buffer)
	if err == nil {
		t.Error("关闭Reader后，Read操作应该返回错误")
	}
}

// TestStreamWithContextCancellation 测试上下文取消功能
func TestStreamWithContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "context cancel test",
	}

	result := ExecuteDockerComposeCommandStream(ctx, options)

	if result.Error != nil {
		t.Skipf("跳过测试，命令启动失败: %v", result.Error)
	}

	if result.Reader == nil {
		t.Fatal("Reader不应该为nil")
	}
	defer result.Reader.Close()

	// 启动读取
	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1024)
		for {
			_, err := result.Reader.Read(buffer)
			if err != nil {
				readDone <- err
				return
			}
		}
	}()

	// 等待一小段时间后取消上下文
	time.Sleep(50 * time.Millisecond)
	cancel()

	// 验证读取操作能在合理时间内结束
	select {
	case err := <-readDone:
		t.Logf("读取结束，错误: %v", err)
	case <-time.After(1 * time.Second):
		t.Error("取消上下文后，读取应该在1秒内结束")
	}
}

// TestStreamErrorHandling 测试流式执行的错误处理
func TestStreamErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		options ExecDockerComposeStreamOptions
		wantErr bool
	}{
		{
			name: "无效路径",
			options: ExecDockerComposeStreamOptions{
				ExecPath:      "/nonexistent/path/that/does/not/exist",
				Args:          []string{"version"},
				OperationName: "invalid path test",
			},
			wantErr: false, // 启动时不会立即报错
		},
		{
			name: "空参数",
			options: ExecDockerComposeStreamOptions{
				ExecPath:      ".",
				Args:          []string{},
				OperationName: "empty args test",
			},
			wantErr: false, // docker compose 没有参数时会显示帮助
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := ExecuteDockerComposeCommandStream(ctx, tt.options)

			if tt.wantErr {
				if result.Error == nil {
					t.Error("期望有错误但没有返回错误")
				}
			} else {
				if result.Error != nil {
					t.Logf("命令启动错误（可能是预期的）: %v", result.Error)
				}
			}

			if result.Reader != nil {
				defer result.Reader.Close()

				// 尝试读取一些数据
				buffer := make([]byte, 512)
				_, err := result.Reader.Read(buffer)
				if err != nil && err != io.EOF {
					t.Logf("读取时出现错误（可能是预期的）: %v", err)
				}
			}
		})
	}
}

// TestStreamDataIntegrity 测试流数据的完整性
func TestStreamDataIntegrity(t *testing.T) {
	options := ExecDockerComposeStreamOptions{
		ExecPath:      ".",
		Args:          []string{"version"},
		OperationName: "data integrity test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := ExecuteDockerComposeCommandStream(ctx, options)

	if result.Error != nil {
		t.Skipf("跳过测试，命令启动失败: %v", result.Error)
	}

	if result.Reader == nil {
		t.Fatal("Reader不应该为nil")
	}
	defer result.Reader.Close()

	// 读取所有数据并验证
	var allData []byte
	buffer := make([]byte, 256)

	for {
		n, err := result.Reader.Read(buffer)
		if n > 0 {
			allData = append(allData, buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("读取时出现错误: %v", err)
			break
		}
	}

	// 验证数据不为空且包含版本信息（如果成功的话）
	if len(allData) > 0 {
		dataStr := string(allData)
		t.Logf("读取到的数据: %s", dataStr)

		// 简单验证包含Docker Compose相关内容
		if strings.Contains(strings.ToLower(dataStr), "version") ||
			strings.Contains(strings.ToLower(dataStr), "docker") ||
			strings.Contains(strings.ToLower(dataStr), "compose") {
			t.Log("数据看起来包含预期的版本信息")
		}
	} else {
		t.Log("没有读取到数据（可能是环境问题）")
	}
}

func TestExecuteDockerComposeCommandSyncsPWDWithExecPath(t *testing.T) {
	tmpRoot := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(tmpRoot)
	if err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(resolvedRoot, "bin")
	srcDir := filepath.Join(resolvedRoot, "src")
	workDir := filepath.Join(resolvedRoot, "apps", "demo")
	for _, d := range []string{binDir, srcDir, workDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// 使用独立编译的 Go 假 docker 二进制，避免 /bin/sh 启动时自动改写 PWD 环境变量
	helperSrc := filepath.Join(srcDir, "main.go")
	helperCode := `package main

import (
	"fmt"
	"os"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	pwd, ok := os.LookupEnv("PWD")
	fmt.Printf("CWD=%s|HAS_PWD=%t|ENV_PWD=%s\n", wd, ok, pwd)
}
`
	if err := os.WriteFile(helperSrc, []byte(helperCode), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeDocker := filepath.Join(binDir, "docker")
	buildCmd := exec.Command("go", "build", "-o", fakeDocker, helperSrc)
	if out, berr := buildCmd.CombinedOutput(); berr != nil {
		t.Fatalf("build helper binary failed: %v\n%s", berr, string(out))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PWD", "/app")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. ExecPath 非空时，同步/流式命令都应将子进程 PWD 同步为 ExecPath 的绝对路径
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      workDir,
		Args:          []string{"config"},
		OperationName: "pwd sync test",
		NeedOutput:    true,
	})
	if res.Error != nil {
		t.Fatalf("ExecuteDockerComposeCommand failed: %v", res.Error)
	}
	wantLine := "CWD=" + workDir + "|HAS_PWD=true|ENV_PWD=" + workDir
	if got := strings.TrimSpace(string(res.Output)); got != wantLine {
		t.Fatalf("sync PWD mismatch: got %q, want %q", got, wantLine)
	}

	streamRes := ExecuteDockerComposeCommandStream(ctx, ExecDockerComposeStreamOptions{
		ExecPath:      workDir,
		Args:          []string{"logs"},
		OperationName: "stream pwd sync test",
	})
	if streamRes.Error != nil {
		t.Fatalf("ExecuteDockerComposeCommandStream failed: %v", streamRes.Error)
	}
	streamOut, rerr := io.ReadAll(streamRes.Reader)
	_ = streamRes.Reader.Close()
	if rerr != nil {
		t.Fatalf("read stream output failed: %v", rerr)
	}
	if exitCode := <-streamRes.ExitCode; exitCode != 0 {
		t.Fatalf("expected exitCode 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(string(streamOut)); got != wantLine {
		t.Fatalf("stream PWD mismatch: got %q, want %q", got, wantLine)
	}

	// 2. ExecPath 为空时保留继承环境中的 PWD
	noDirRes := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      "",
		Args:          []string{"ls"},
		OperationName: "inherit pwd test",
		NeedOutput:    true,
	})
	if noDirRes.Error != nil {
		t.Fatalf("ExecuteDockerComposeCommand without ExecPath failed: %v", noDirRes.Error)
	}
	if !strings.Contains(string(noDirRes.Output), "|HAS_PWD=true|ENV_PWD=/app") {
		t.Fatalf("expected inherited ENV_PWD=/app when ExecPath is empty, got %q", string(noDirRes.Output))
	}
}
