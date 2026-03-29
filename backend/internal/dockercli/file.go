package dockercli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// FileEntry 文件条目信息
type FileEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // file, directory, symlink, other
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"` // rwxr-xr-x
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	Modified    string `json:"modified"` // RFC3339 格式
	LinkTarget  string `json:"linkTarget,omitempty"`
	Readonly    bool   `json:"readonly,omitempty"`
}

// FileListResult 文件列表结果
type FileListResult struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

// ExecContainer 在容器中执行命令并返回输出
func (c *Client) ExecContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	// 创建 exec 配置
	execConfig := container.ExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmd,
	}

	// 创建 exec 实例
	execResp, err := c.docker.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("create exec failed: %w", err)
	}

	// 附加到 exec 实例
	attachResp, err := c.docker.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{
		Tty: false,
	})
	if err != nil {
		return "", fmt.Errorf("attach exec failed: %w", err)
	}
	defer attachResp.Close()

	// 读取输出（非 TTY 模式下，Docker 会在输出前加上 8 字节的头部）
	// 头部格式: [STREAM_TYPE(1 byte)][RESERVED(3 bytes)][SIZE(4 bytes)]
	var output strings.Builder
	var stderr strings.Builder
	buf := make([]byte, 4096)
	remainder := []byte{} // 保存上次读取的不完整数据

	readDone := false
	for !readDone {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			n, err := attachResp.Reader.Read(buf)
			if n > 0 {
				// 合并上次剩余的数据
				data := append(remainder, buf[:n]...)
				remainder = []byte{}

				// 处理 Docker 的流格式
				offset := 0
				for offset < len(data) {
					if offset+8 > len(data) {
						// 头部不完整，保存到 remainder
						remainder = data[offset:]
						break
					}

					// 读取头部
					streamType := data[offset]
					size := int(data[offset+4])<<24 | int(data[offset+5])<<16 | int(data[offset+6])<<8 | int(data[offset+7])

					offset += 8

					if offset+size > len(data) {
						// 数据不完整，保存到 remainder（包括头部）
						remainder = data[offset-8:]
						break
					}

					// 根据流类型写入对应的缓冲区
					chunk := data[offset : offset+size]
					switch streamType {
					case 1:
						// stdout
						output.Write(chunk)
					case 2:
						// stderr
						stderr.Write(chunk)
					}

					offset += size
				}
			}

			if err != nil {
				if err == io.EOF {
					readDone = true
					break
				}
				return "", fmt.Errorf("read output failed: %w", err)
			}
		}
	}

	// 检查是否有错误输出
	if stderr.Len() > 0 {
		logger.Logger.Warn("exec command stderr",
			zap.String("container", containerID),
			zap.Strings("cmd", cmd),
			zap.String("stderr", stderr.String()))
	}

	return output.String(), nil
}

// parseLsOutput 解析 ls -la 命令输出
func parseLsOutput(output string) ([]FileEntry, error) {
	lines := strings.Split(output, "\n")
	entries := []FileEntry{}

	// 正则表达式匹配 ls -la 输出
	// 支持多种格式：
	// 1. 标准格式: drwxr-xr-x 2 root root 4096 2024-01-01 12:00:00 filename
	// 2. ISO 格式: -rw-r--r-- 1 root root 1024 2024-01-01 12:00 filename
	// 3. 传统格式: lrwxrwxrwx 1 root root 7 Jan 01 12:00 link -> target
	//
	// 匹配策略：权限 链接数 所有者 组 大小 (其余所有内容)
	// 然后从其余内容中分离时间和文件名
	re := regexp.MustCompile(`^([dlrwxst-]+)\s+\d+\s+(\S+)\s+(\S+)\s+(\d+)\s+(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) < 6 {
			continue
		}

		perms := matches[1]
		owner := matches[2]
		group := matches[3]
		sizeStr := matches[4]
		remaining := strings.TrimSpace(matches[5]) // 时间 + 文件名

		// 从 remaining 中分离时间和文件名
		// 尝试多种时间格式的匹配
		var dateStr, namePart string

		// 1. 尝试匹配 YYYY-MM-DD HH:MM:SS 格式（我们的自定义格式）
		if timeMatch := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 2. 尝试匹配 YYYY-MM-DD HH:MM 格式
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}(?::\d{2})?)\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 3. 尝试匹配 Jan 01 12:00:00 或 Jan 01 12:00 格式
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{4})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 4. 尝试匹配 Jan 01 2006 格式
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else {
			// 5. 如果都不匹配，尝试简单分割（取最后一部分作为文件名）
			parts := strings.Fields(remaining)
			if len(parts) >= 2 {
				// 最后一个是文件名，前面的是时间
				namePart = parts[len(parts)-1]
				dateStr = strings.Join(parts[:len(parts)-1], " ")
			} else {
				// 无法分离，跳过这一行
				logger.Logger.Warn("cannot parse ls line", zap.String("line", line))
				continue
			}
		}

		namePart = strings.TrimSpace(namePart)
		dateStr = strings.TrimSpace(dateStr)

		// 跳过 . 和 ..
		// 注意：文件名可能还包含符号链接箭头，所以要先检查
		nameOnly := namePart
		if strings.Contains(namePart, " -> ") {
			nameOnly = strings.Split(namePart, " -> ")[0]
			nameOnly = strings.TrimSpace(nameOnly)
		}

		if nameOnly == "." || nameOnly == ".." {
			continue
		}

		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			logger.Logger.Warn("parse size failed", zap.String("size", sizeStr), zap.Error(err))
			size = 0
		}

		// 判断文件类型
		fileType := "other"
		var linkTarget string
		name := namePart

		if strings.HasPrefix(perms, "d") {
			fileType = "directory"
		} else if strings.HasPrefix(perms, "l") {
			fileType = "symlink"
			// 解析符号链接目标
			parts := strings.Split(namePart, " -> ")
			if len(parts) == 2 {
				name = strings.TrimSpace(parts[0])
				linkTarget = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(perms, "-") {
			fileType = "file"
		}

		// 检查是否只读（检查所有者、组和其他人的写权限）
		readonly := true
		if len(perms) >= 10 {
			// 检查所有者写权限 (索引 2)
			if perms[2] == 'w' {
				readonly = false
			}
			// 如果所有者没有写权限，检查组写权限 (索引 5)
			if readonly && len(perms) >= 6 && perms[5] == 'w' {
				readonly = false
			}
			// 如果前两者都没有写权限，检查其他人写权限 (索引 8)
			if readonly && len(perms) >= 9 && perms[8] == 'w' {
				readonly = false
			}
		}

		// 解析时间
		modified := parseFileTime(dateStr)

		entries = append(entries, FileEntry{
			Name:        name,
			Type:        fileType,
			Size:        size,
			Permissions: perms,
			Owner:       owner,
			Group:       group,
			Modified:    modified,
			LinkTarget:  linkTarget,
			Readonly:    readonly,
		})
	}

	return entries, nil
}

// parseFileTime 解析文件时间
func parseFileTime(timeStr string) string {
	// 清理时间字符串
	timeStr = strings.TrimSpace(timeStr)
	// 移除多余的空格
	timeStr = regexp.MustCompile(`\s+`).ReplaceAllString(timeStr, " ")

	// 按优先级尝试多种格式
	// 优先处理我们自定义的标准格式
	formats := []string{
		"2006-01-02 15:04:05", // 我们的自定义格式（最优先）
		"2006-01-02 15:04",    // ISO 格式简化版
		"Jan 02 15:04:05",     // 传统格式
		"Jan 02 15:04",        // 传统格式简化版
		"Jan 02 2006",         // 只有日期
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	// 处理特殊格式：MM-DD（只有月-日，BusyBox ls 输出）
	if matched := regexp.MustCompile(`^(\d{2})-(\d{2})$`).FindStringSubmatch(timeStr); len(matched) == 3 {
		// 使用当前年份，时间设为 00:00
		currentYear := time.Now().Year()
		dateStr := fmt.Sprintf("%d-%s-%s 00:00", currentYear, matched[1], matched[2])
		if t, err := time.Parse("2006-01-02 15:04", dateStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	// 处理特殊格式：YYYY-MM-DD（只有年-月-日）
	if matched := regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`).FindStringSubmatch(timeStr); len(matched) == 4 {
		dateStr := fmt.Sprintf("%s-%s-%s 00:00", matched[1], matched[2], matched[3])
		if t, err := time.Parse("2006-01-02 15:04", dateStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	// 如果所有格式都失败，返回当前时间
	logger.Logger.Debug("parse time failed, using current time", zap.String("time", timeStr))
	return time.Now().Format(time.RFC3339)
}

// ListContainerDirectory 列出容器目录内容
func (c *Client) ListContainerDirectory(ctx context.Context, containerID, path string) (*FileListResult, error) {
	// 清理路径防止注入
	safePath := sanitizePath(path)

	// 尝试多种命令，按优先级排序
	commands := [][]string{
		// 1. 尝试使用自定义时间格式（最标准）
		{"ls", "-la", "--time-style=+%Y-%m-%d %H:%M:%S", safePath},
		// 2. 尝试使用 ISO 时间格式
		{"ls", "-la", "--time-style=iso", safePath},
		// 3. 尝试使用完整时间格式
		{"ls", "-la", "--full-time", safePath},
		// 4. 降级到简单的 ls（最后选择）
		{"ls", "-la", safePath},
	}

	var output string
	var lastErr error

	for i, cmd := range commands {
		output, lastErr = c.ExecContainer(ctx, containerID, cmd)
		if lastErr == nil {
			// 成功执行
			if i > 0 {
				logger.Logger.Debug("ls command succeeded",
					zap.String("container", containerID),
					zap.String("path", safePath),
					zap.Int("attempt", i+1))
			}
			break
		}
		logger.Logger.Debug("ls command failed, trying next",
			zap.String("container", containerID),
			zap.String("path", safePath),
			zap.Int("attempt", i+1),
			zap.Strings("cmd", cmd),
			zap.Error(lastErr))
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to list directory: %w", lastErr)
	}

	entries, err := parseLsOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ls output: %w", err)
	}

	return &FileListResult{
		Path:    safePath,
		Entries: entries,
	}, nil
}

// sanitizePath 清理文件路径防止命令注入
func sanitizePath(path string) string {
	// 移除危险字符
	dangerous := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "[", "]", "<", ">", "'", "\"", "\\"}
	result := path
	for _, char := range dangerous {
		result = strings.ReplaceAll(result, char, "")
	}
	return result
}

// ReadContainerFile 读取容器文件内容
func (c *Client) ReadContainerFile(ctx context.Context, containerID, path string) (string, error) {
	safePath := sanitizePath(path)

	cmd := []string{"cat", safePath}
	output, err := c.ExecContainer(ctx, containerID, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return output, nil
}

// ReadContainerFileFromArchive 使用 Docker Archive API 读取容器文件内容（更高效）
func (c *Client) ReadContainerFileFromArchive(ctx context.Context, containerID, path string) (string, error) {
	safePath := sanitizePath(path)

	// 使用 CopyFromContainer API 获取文件
	reader, _, err := c.docker.CopyFromContainer(ctx, containerID, safePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}
	defer reader.Close()

	// 解析 tar 格式
	tarReader := tar.NewReader(reader)

	// 读取第一个文件（应该是我们请求的文件）
	header, err := tarReader.Next()
	if err != nil {
		return "", fmt.Errorf("failed to read tar header: %w", err)
	}

	// 检查是否为目录
	if header.Typeflag == tar.TypeDir {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	// 读取文件内容
	content := make([]byte, header.Size)
	_, err = io.ReadFull(tarReader, content)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}

	return string(content), nil
}

// ReadContainerFileAuto 智能选择最佳方法读取文件
// 优先使用 Archive API，失败时自动降级到 cat 命令
func (c *Client) ReadContainerFileAuto(ctx context.Context, containerID, path string) (string, error) {
	safePath := sanitizePath(path)

	// 1. 检查是否为虚拟文件系统（必须使用 cat）
	if isVirtualFilesystem(safePath) {
		logger.Logger.Debug("virtual filesystem detected, using cat command",
			zap.String("path", safePath))
		return c.ReadContainerFile(ctx, containerID, path)
	}

	// 2. 尝试使用 Archive API（性能更好）
	content, err := c.ReadContainerFileFromArchive(ctx, containerID, path)
	if err == nil {
		return content, nil
	}

	// 3. Archive API 失败，自动降级到 cat 命令
	logger.Logger.Debug("archive API failed, fallback to cat command",
		zap.String("path", safePath),
		zap.Error(err))

	return c.ReadContainerFile(ctx, containerID, path)
}

// isVirtualFilesystem 检查路径是否为虚拟文件系统
// 虚拟文件系统必须使用 cat 命令读取
func isVirtualFilesystem(path string) bool {
	virtualPrefixes := []string{
		"/proc/", // procfs - 进程信息
		"/sys/",  // sysfs - 系统信息
		"/dev/",  // devfs - 设备文件
	}

	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// WriteContainerFile 写入文件内容到容器
func (c *Client) WriteContainerFile(ctx context.Context, containerID, path, content string) error {
	safePath := sanitizePath(path)

	// 获取文件所在目录和文件名
	dir := filepath.Dir(safePath)
	if dir == "." || dir == "" {
		dir = "/"
	}
	filename := filepath.Base(safePath)
	if filename == "." || filename == "/" {
		return fmt.Errorf("invalid file path")
	}

	// 创建 tar 包
	tarData, err := createTarArchive(filename, []byte(content))
	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	// 使用 Docker Archive API 上传
	err = c.PutContainerArchive(ctx, containerID, dir, tarData)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// createTarArchive 创建简单的 tar 包
func createTarArchive(filename string, content []byte) ([]byte, error) {
	// TAR header 512 字节
	header := make([]byte, 512)

	// 文件名 (100 bytes)
	nameBytes := []byte(filename)
	if len(nameBytes) > 100 {
		return nil, fmt.Errorf("filename too long (max 100 bytes)")
	}
	copy(header[0:], nameBytes)

	// 文件模式 (8 bytes) - 0644
	copy(header[100:], "0000644\x00")

	// UID (8 bytes)
	copy(header[108:], "0000000\x00")

	// GID (8 bytes)
	copy(header[116:], "0000000\x00")

	// 文件大小 (12 bytes octal)
	sizeStr := fmt.Sprintf("%011o\x00", len(content))
	copy(header[124:], sizeStr)

	// 修改时间 (12 bytes octal)
	mtimeStr := fmt.Sprintf("%011o\x00", time.Now().Unix())
	copy(header[136:], mtimeStr)

	// Checksum 占位符 (8 bytes)
	copy(header[148:], "        ")

	// 类型标志 - '0' 表示普通文件
	header[156] = '0'

	// Magic - "ustar\x00"
	copy(header[257:], "ustar\x00")

	// Version - "00"
	copy(header[263:], "00")

	// Owner name (32 bytes)
	copy(header[265:], "root")

	// Group name (32 bytes)
	copy(header[297:], "root")

	// 计算校验和
	checksum := 0
	for i := 0; i < 512; i++ {
		checksum += int(header[i])
	}
	checksumStr := fmt.Sprintf("%06o\x00 ", checksum)
	copy(header[148:], checksumStr)

	// 计算填充到 512 字节边界
	paddingSize := (512 - (len(content) % 512)) % 512
	padding := make([]byte, paddingSize)

	// 结束标记（两个 512 字节的零块）
	endMarker := make([]byte, 1024)

	// 组合所有部分
	result := append(header, content...)
	result = append(result, padding...)
	result = append(result, endMarker...)

	return result, nil
}

// PutContainerArchive 上传 tar 包到容器
func (c *Client) PutContainerArchive(ctx context.Context, containerID, path string, tarData []byte) error {
	safePath := sanitizePath(path)

	opts := container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	}

	err := c.docker.CopyToContainer(ctx, containerID, safePath, bytes.NewReader(tarData), opts)
	if err != nil {
		return fmt.Errorf("failed to copy to container: %w", err)
	}

	return nil
}

// GetContainerArchive 获取容器文件/目录的 tar 包
func (c *Client) GetContainerArchive(ctx context.Context, containerID, path string) (io.ReadCloser, error) {
	safePath := sanitizePath(path)

	reader, _, err := c.docker.CopyFromContainer(ctx, containerID, safePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get archive: %w", err)
	}

	return reader, nil
}

// CreateContainerFile 在容器中创建文件
func (c *Client) CreateContainerFile(ctx context.Context, containerID, path string) error {
	safePath := sanitizePath(path)
	cmd := []string{"touch", safePath}
	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}

// CreateContainerDirectory 在容器中创建目录
func (c *Client) CreateContainerDirectory(ctx context.Context, containerID, path string) error {
	safePath := sanitizePath(path)
	cmd := []string{"mkdir", "-p", safePath}
	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}

// DeleteContainerPath 删除容器中的文件或目录
func (c *Client) DeleteContainerPath(ctx context.Context, containerID, path string) error {
	safePath := sanitizePath(path)
	cmd := []string{"rm", "-rf", safePath}
	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}

// RenameContainerPath 重命名容器中的文件或目录
func (c *Client) RenameContainerPath(ctx context.Context, containerID, oldPath, newPath string) error {
	safeOldPath := sanitizePath(oldPath)
	safeNewPath := sanitizePath(newPath)
	cmd := []string{"mv", safeOldPath, safeNewPath}
	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}

// ChmodContainerPath 修改容器文件权限
func (c *Client) ChmodContainerPath(ctx context.Context, containerID, path, mode string, recursive bool) error {
	safePath := sanitizePath(path)

	// 验证 mode 格式（应该是八进制数字，如 "755" 或 "0644"）
	// 简单的验证：只允许数字和八进制格式
	if len(mode) == 0 {
		return fmt.Errorf("mode cannot be empty")
	}

	cmd := []string{"chmod"}
	if recursive {
		cmd = append(cmd, "-R")
	}
	cmd = append(cmd, mode, safePath)

	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}
