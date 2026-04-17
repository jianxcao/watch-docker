package dockercli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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

	// 通过 ContainerExecInspect 检查命令的退出码
	inspectResp, err := c.docker.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		logger.Logger.Warn("exec inspect failed",
			zap.String("container", containerID),
			zap.Strings("cmd", cmd),
			zap.Error(err))
	} else if inspectResp.ExitCode != 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr == "" {
			stderrStr = fmt.Sprintf("exit code %d", inspectResp.ExitCode)
		}
		return "", fmt.Errorf("command exited with code %d: %s", inspectResp.ExitCode, stderrStr)
	}

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
		// 尝试多种时间格式的匹配（优先匹配更长/更具体的格式，避免贪婪匹配截断文件名）
		var dateStr, namePart string

		if timeMatch := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?\s+[+-]\d{4})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 1. --full-time 格式: 2024-01-01 12:00:00.000000000 +0000 或 2024-01-01 12:00:00 +0000
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 2. 自定义格式: 2024-01-01 12:00:00
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 3. ISO 格式: 2024-01-01 12:00
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}(?::\d{2})?)\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 4. 传统格式: Jan 01 12:00 或 Jan 01 12:00:00
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{4})\s+(.+)$`).FindStringSubmatch(remaining); len(timeMatch) == 3 {
			// 5. 年份格式: Jan 01 2006
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else {
			// 6. 兜底：取最后一部分作为文件名
			parts := strings.Fields(remaining)
			if len(parts) >= 2 {
				namePart = parts[len(parts)-1]
				dateStr = strings.Join(parts[:len(parts)-1], " ")
			} else {
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

	// 处理 --full-time 格式中的纳秒部分：
	// "2024-01-01 12:00:00.000000000 +0000" → "2024-01-01 12:00:00 +0000"
	timeStr = regexp.MustCompile(`(\d{2}:\d{2}:\d{2})\.\d+ `).ReplaceAllString(timeStr, "${1} ")

	formats := []string{
		"2006-01-02 15:04:05 -0700", // --full-time 格式（带时区）
		"2006-01-02 15:04:05",       // 自定义格式
		"2006-01-02 15:04",          // ISO 格式
		"Jan 02 15:04:05",           // 传统格式
		"Jan 02 15:04",              // 传统格式简化版
		"Jan 02 2006",               // 只有日期
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
// 优先使用 exec ls 命令，全部失败时降级到 Docker Archive API
func (c *Client) ListContainerDirectory(ctx context.Context, containerID, path string) (*FileListResult, error) {
	safePath := sanitizePath(path)

	commands := [][]string{
		{"ls", "-la", "--time-style=+%Y-%m-%d %H:%M:%S", safePath},
		{"ls", "-la", "--time-style=iso", safePath},
		{"ls", "-la", "--full-time", safePath},
		{"ls", "-la", safePath},
	}

	var output string
	var lastErr error

	for i, cmd := range commands {
		output, lastErr = c.ExecContainer(ctx, containerID, cmd)
		if lastErr == nil {
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
		// 所有 ls 命令都失败，降级到 Docker Archive API
		logger.Logger.Info("all ls commands failed, falling back to archive API",
			zap.String("container", containerID),
			zap.String("path", safePath),
			zap.Error(lastErr))
		return c.listContainerDirectoryFromArchive(ctx, containerID, safePath)
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

// listContainerDirectoryFromArchive 通过 Docker Archive API (CopyFromContainer) 列出目录
// 不依赖容器内的任何 shell 命令，适用于 distroless / scratch 等无 shell 镜像
func (c *Client) listContainerDirectoryFromArchive(ctx context.Context, containerID, path string) (*FileListResult, error) {
	reader, _, err := c.docker.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, fmt.Errorf("archive API failed: %w", err)
	}
	defer reader.Close()

	tarReader := tar.NewReader(reader)
	entries := []FileEntry{}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry failed: %w", err)
		}

		name := strings.TrimSuffix(header.Name, "/")

		// CopyFromContainer 返回的 tar 以目标目录作为根
		// 比如请求 /app，tar 中的条目是: app/, app/file1, app/sub/file2
		// 第一个条目是目录自身（根），跳过
		// 只保留第一层子项（不含嵌套子目录的内容）
		parts := strings.Split(name, "/")
		if len(parts) <= 1 {
			// 根条目（目录自身），跳过
			continue
		}
		if len(parts) > 2 {
			// 嵌套子项，跳过（只要第一层）
			continue
		}

		childName := parts[1]
		if childName == "" || childName == "." || childName == ".." {
			continue
		}

		fileType := "other"
		linkTarget := ""
		switch header.Typeflag {
		case tar.TypeDir:
			fileType = "directory"
		case tar.TypeReg, tar.TypeRegA:
			fileType = "file"
		case tar.TypeSymlink:
			fileType = "symlink"
			linkTarget = header.Linkname
		}

		mode := header.FileInfo().Mode()
		perms := formatPermissions(mode, header.Typeflag)
		readonly := mode.Perm()&0222 == 0

		owner := header.Uname
		group := header.Gname
		if owner == "" {
			owner = fmt.Sprintf("%d", header.Uid)
		}
		if group == "" {
			group = fmt.Sprintf("%d", header.Gid)
		}

		entries = append(entries, FileEntry{
			Name:        childName,
			Type:        fileType,
			Size:        header.Size,
			Permissions: perms,
			Owner:       owner,
			Group:       group,
			Modified:    header.ModTime.Format(time.RFC3339),
			LinkTarget:  linkTarget,
			Readonly:    readonly,
		})
	}

	return &FileListResult{
		Path:    path,
		Entries: entries,
	}, nil
}

// formatPermissions 将 os.FileMode 转为 ls 风格的权限字符串（如 drwxr-xr-x）
func formatPermissions(mode os.FileMode, tarType byte) string {
	var buf [10]byte

	switch tarType {
	case tar.TypeDir:
		buf[0] = 'd'
	case tar.TypeSymlink:
		buf[0] = 'l'
	default:
		buf[0] = '-'
	}

	const rwx = "rwx"
	perm := mode.Perm()
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			buf[1+i] = rwx[i%3]
		} else {
			buf[1+i] = '-'
		}
	}

	return string(buf[:])
}

// sanitizePath 清理文件路径防止命令注入和路径穿越
func sanitizePath(path string) string {
	dangerous := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "[", "]", "<", ">", "'", "\"", "\\"}
	result := path
	for _, char := range dangerous {
		result = strings.ReplaceAll(result, char, "")
	}

	result = filepath.Clean(result)

	if !strings.HasPrefix(result, "/") {
		result = "/" + result
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

	if len(mode) == 0 || len(mode) > 4 {
		return fmt.Errorf("invalid mode format")
	}
	for _, ch := range mode {
		if ch < '0' || ch > '7' {
			return fmt.Errorf("invalid mode: must be octal digits (0-7)")
		}
	}

	cmd := []string{"chmod"}
	if recursive {
		cmd = append(cmd, "-R")
	}
	cmd = append(cmd, mode, safePath)

	_, err := c.ExecContainer(ctx, containerID, cmd)
	return err
}
