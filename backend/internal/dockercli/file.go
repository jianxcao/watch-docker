package dockercli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// MaxEditableFileSize 限制在线查看/编辑的单文件最大体积（5MB），防止读取超大文件导致 OOM
const MaxEditableFileSize int64 = 5 * 1024 * 1024

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

// cleanContainerPath 严格规范化容器内 POSIX 路径。
// 不再静默剥离特殊字符（防止 "/;" 变成 "/" 后触发危险操作），因为所有命令均通过参数数组执行，不经过 shell 解析。
func cleanContainerPath(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("invalid path: null byte")
	}
	cleaned := path.Clean("/" + trimmed)
	return cleaned, nil
}

// ExecContainer 在容器中执行命令并返回 stdout 输出
func (c *Client) ExecContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	execConfig := client.ExecCreateOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
		Cmd:          cmd,
	}

	execResp, err := c.docker.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("create exec failed: %w", err)
	}

	attachResp, err := c.docker.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{
		TTY: false,
	})
	if err != nil {
		return "", fmt.Errorf("attach exec failed: %w", err)
	}
	defer attachResp.Close()

	// 若上下文取消则主动关闭劫持连接以解除 StdCopy 阻塞
	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		select {
		case <-ctx.Done():
			attachResp.Close()
		case <-doneCh:
		}
	}()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("read output failed: %w", err)
	}

	inspectResp, err := c.docker.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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
		logger.Logger.Debug("exec command stderr",
			zap.String("container", containerID),
			zap.Strings("cmd", cmd),
			zap.String("stderr", stderr.String()))
	}

	return stdout.String(), nil
}

// parseLsOutput 解析 ls -la 命令输出
func parseLsOutput(output string) ([]FileEntry, error) {
	lines := strings.Split(output, "\n")
	entries := []FileEntry{}

	re := regexp.MustCompile(`^([dlrwxst-]+)\s+\d+\s+(\S+)\s+(\S+)\s+(\d+)\s+(.+)$`)
	reFullTime := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?\s+[+-]\d{4})\s+(.+)$`)
	reCustomTime := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.+)$`)
	reIsoTime := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})\s+(.+)$`)
	reTradTime := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}(?::\d{2})?)\s+(.+)$`)
	reYearTime := regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{4})\s+(.+)$`)

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
		remaining := strings.TrimSpace(matches[5])

		var dateStr, namePart string

		if timeMatch := reFullTime.FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := reCustomTime.FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := reIsoTime.FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := reTradTime.FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else if timeMatch := reYearTime.FindStringSubmatch(remaining); len(timeMatch) == 3 {
			dateStr = timeMatch[1]
			namePart = timeMatch[2]
		} else {
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

		nameOnly := namePart
		if strings.Contains(namePart, " -> ") {
			nameOnly = strings.TrimSpace(strings.Split(namePart, " -> ")[0])
		}

		if nameOnly == "." || nameOnly == ".." {
			continue
		}

		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			logger.Logger.Warn("parse size failed", zap.String("size", sizeStr), zap.Error(err))
			size = 0
		}

		fileType := "other"
		var linkTarget string
		name := namePart

		if strings.HasPrefix(perms, "d") {
			fileType = "directory"
		} else if strings.HasPrefix(perms, "l") {
			fileType = "symlink"
			parts := strings.Split(namePart, " -> ")
			if len(parts) == 2 {
				name = strings.TrimSpace(parts[0])
				linkTarget = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(perms, "-") {
			fileType = "file"
		}

		readonly := true
		if len(perms) >= 10 {
			if perms[2] == 'w' || perms[5] == 'w' || perms[8] == 'w' {
				readonly = false
			}
		}

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

var (
	reSpaces   = regexp.MustCompile(`\s+`)
	reNano     = regexp.MustCompile(`(\d{2}:\d{2}:\d{2})\.\d+ `)
	reMMDD     = regexp.MustCompile(`^(\d{2})-(\d{2})$`)
	reYYYYMMDD = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
)

// parseFileTime 解析文件时间
func parseFileTime(timeStr string) string {
	timeStr = strings.TrimSpace(timeStr)
	timeStr = reSpaces.ReplaceAllString(timeStr, " ")
	timeStr = reNano.ReplaceAllString(timeStr, "${1} ")

	formats := []string{
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"Jan 02 15:04:05",
		"Jan 02 15:04",
		"Jan 02 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	if matched := reMMDD.FindStringSubmatch(timeStr); len(matched) == 3 {
		currentYear := time.Now().Year()
		dateStr := fmt.Sprintf("%d-%s-%s 00:00", currentYear, matched[1], matched[2])
		if t, err := time.Parse("2006-01-02 15:04", dateStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	if matched := reYYYYMMDD.FindStringSubmatch(timeStr); len(matched) == 4 {
		dateStr := fmt.Sprintf("%s-%s-%s 00:00", matched[1], matched[2], matched[3])
		if t, err := time.Parse("2006-01-02 15:04", dateStr); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	logger.Logger.Debug("parse time failed, using current time", zap.String("time", timeStr))
	return time.Now().Format(time.RFC3339)
}

// ListContainerDirectory 列出容器目录内容
// 优先使用 exec ls 命令，全部失败时降级到 Docker Archive API
func (c *Client) ListContainerDirectory(ctx context.Context, containerID, rawPath string) (*FileListResult, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return nil, err
	}

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
			break
		}
		logger.Logger.Debug("ls command failed, trying next",
			zap.String("container", containerID),
			zap.String("path", safePath),
			zap.Int("attempt", i+1),
			zap.Error(lastErr))
	}

	if lastErr != nil {
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
func (c *Client) listContainerDirectoryFromArchive(ctx context.Context, containerID, safePath string) (*FileListResult, error) {
	copyRes, err := c.docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: safePath})
	if err != nil {
		return nil, fmt.Errorf("archive API failed: %w", err)
	}
	defer copyRes.Content.Close()

	tarReader := tar.NewReader(copyRes.Content)
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
		parts := strings.Split(name, "/")
		if len(parts) <= 1 || len(parts) > 2 {
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
		Path:    safePath,
		Entries: entries,
	}, nil
}

// formatPermissions 将 os.FileMode 转为 ls 风格的权限字符串
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

// ReadContainerFile 读取容器文件内容
func (c *Client) ReadContainerFile(ctx context.Context, containerID, rawPath string) (string, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return "", err
	}

	cmd := []string{"cat", safePath}
	output, err := c.ExecContainer(ctx, containerID, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if int64(len(output)) > MaxEditableFileSize {
		return "", fmt.Errorf("文件过大（超过 %d MB 在线编辑限制），请使用下载功能", MaxEditableFileSize/(1024*1024))
	}

	return output, nil
}

// ReadContainerFileFromArchive 使用 Docker Archive API 读取容器文件内容（限制体积防 OOM）
func (c *Client) ReadContainerFileFromArchive(ctx context.Context, containerID, rawPath string) (string, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return "", err
	}

	copyRes, err := c.docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: safePath})
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}
	defer copyRes.Content.Close()

	tarReader := tar.NewReader(copyRes.Content)

	header, err := tarReader.Next()
	if err != nil {
		return "", fmt.Errorf("failed to read tar header: %w", err)
	}

	if header.Typeflag == tar.TypeDir {
		return "", fmt.Errorf("path is a directory, not a file")
	}
	if header.Size > MaxEditableFileSize {
		return "", fmt.Errorf("文件过大（超过 %d MB 在线编辑限制），请使用下载功能", MaxEditableFileSize/(1024*1024))
	}

	limited := io.LimitReader(tarReader, MaxEditableFileSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}
	if int64(len(content)) > MaxEditableFileSize {
		return "", fmt.Errorf("文件过大（超过 %d MB 在线编辑限制），请使用下载功能", MaxEditableFileSize/(1024*1024))
	}

	return string(content), nil
}

// ReadContainerFileAuto 智能选择最佳方法读取文件
func (c *Client) ReadContainerFileAuto(ctx context.Context, containerID, rawPath string) (string, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return "", err
	}

	if isVirtualFilesystem(safePath) {
		logger.Logger.Debug("virtual filesystem detected, using cat command",
			zap.String("path", safePath))
		return c.ReadContainerFile(ctx, containerID, safePath)
	}

	content, err := c.ReadContainerFileFromArchive(ctx, containerID, safePath)
	if err == nil {
		return content, nil
	}

	// 若明确是文件过大限制，直接返回错误，避免再降级 cat 读入内存
	if strings.Contains(err.Error(), "在线编辑限制") {
		return "", err
	}

	logger.Logger.Debug("archive API failed, fallback to cat command",
		zap.String("path", safePath),
		zap.Error(err))

	return c.ReadContainerFile(ctx, containerID, safePath)
}

func isVirtualFilesystem(p string) bool {
	virtualPrefixes := []string{
		"/proc/",
		"/sys/",
		"/dev/",
	}

	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// WriteContainerFile 写入文件内容到容器（使用标准 archive/tar）
func (c *Client) WriteContainerFile(ctx context.Context, containerID, rawPath, content string) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}
	if safePath == "/" {
		return fmt.Errorf("invalid file path")
	}

	dir := path.Dir(safePath)
	if dir == "." || dir == "" {
		dir = "/"
	}
	filename := path.Base(safePath)
	if filename == "." || filename == "/" {
		return fmt.Errorf("invalid file path")
	}

	return c.PutSingleFileStream(ctx, containerID, dir, filename, int64(len(content)), strings.NewReader(content))
}

// PutSingleFileStream 使用 io.Pipe + stdlib archive/tar 流式上传单个文件到容器，避免大文件全量读入内存
func (c *Client) PutSingleFileStream(ctx context.Context, containerID, targetDir, filename string, size int64, src io.Reader) error {
	safeDir, err := cleanContainerPath(targetDir)
	if err != nil {
		return err
	}
	cleanName := path.Base(strings.TrimSpace(filename))
	if cleanName == "" || cleanName == "." || cleanName == ".." || cleanName == "/" {
		return fmt.Errorf("invalid filename")
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		tw := tar.NewWriter(pw)
		hdr := &tar.Header{
			Name:    cleanName,
			Mode:    0644,
			Size:    size,
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = pw.CloseWithError(err)
			errCh <- err
			return
		}
		if _, err := io.CopyN(tw, src, size); err != nil && err != io.EOF {
			_ = tw.Close()
			_ = pw.CloseWithError(err)
			errCh <- err
			return
		}
		if err := tw.Close(); err != nil {
			_ = pw.CloseWithError(err)
			errCh <- err
			return
		}
		_ = pw.Close()
		errCh <- nil
	}()

	opts := client.CopyToContainerOptions{
		DestinationPath:           safeDir,
		Content:                   pr,
		AllowOverwriteDirWithFile: true,
	}

	_, copyErr := c.docker.CopyToContainer(ctx, containerID, opts)
	_ = pr.Close()
	pipeErr := <-errCh

	if copyErr != nil {
		return fmt.Errorf("failed to copy to container: %w", copyErr)
	}
	if pipeErr != nil {
		return fmt.Errorf("failed to build tar stream: %w", pipeErr)
	}
	return nil
}

// PutContainerArchive 上传 tar 流到容器
func (c *Client) PutContainerArchive(ctx context.Context, containerID, rawPath string, tarData []byte) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}

	opts := client.CopyToContainerOptions{
		DestinationPath:           safePath,
		Content:                   bytes.NewReader(tarData),
		AllowOverwriteDirWithFile: true,
	}

	_, err = c.docker.CopyToContainer(ctx, containerID, opts)
	if err != nil {
		return fmt.Errorf("failed to copy to container: %w", err)
	}

	return nil
}

// GetContainerArchive 获取容器文件/目录的 tar 包
func (c *Client) GetContainerArchive(ctx context.Context, containerID, rawPath string) (io.ReadCloser, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return nil, err
	}

	copyRes, err := c.docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: safePath})
	if err != nil {
		return nil, fmt.Errorf("failed to get archive: %w", err)
	}

	return copyRes.Content, nil
}

// CreateContainerFile 在容器中创建文件
func (c *Client) CreateContainerFile(ctx context.Context, containerID, rawPath string) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}
	if safePath == "/" {
		return fmt.Errorf("invalid file path")
	}
	cmd := []string{"touch", safePath}
	_, err = c.ExecContainer(ctx, containerID, cmd)
	return err
}

// CreateContainerDirectory 在容器中创建目录
func (c *Client) CreateContainerDirectory(ctx context.Context, containerID, rawPath string) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}
	if safePath == "/" {
		return fmt.Errorf("invalid directory path")
	}
	cmd := []string{"mkdir", "-p", safePath}
	_, err = c.ExecContainer(ctx, containerID, cmd)
	return err
}

// DeleteContainerPath 删除容器中的文件或目录
func (c *Client) DeleteContainerPath(ctx context.Context, containerID, rawPath string) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}
	if safePath == "/" {
		return fmt.Errorf("refusing to delete root directory /")
	}
	cmd := []string{"rm", "-rf", safePath}
	_, err = c.ExecContainer(ctx, containerID, cmd)
	return err
}

// RenameContainerPath 重命名容器中的文件或目录
func (c *Client) RenameContainerPath(ctx context.Context, containerID, oldPath, newPath string) error {
	safeOldPath, err := cleanContainerPath(oldPath)
	if err != nil {
		return err
	}
	safeNewPath, err := cleanContainerPath(newPath)
	if err != nil {
		return err
	}
	if safeOldPath == "/" || safeNewPath == "/" {
		return fmt.Errorf("refusing to rename root directory /")
	}
	cmd := []string{"mv", safeOldPath, safeNewPath}
	_, err = c.ExecContainer(ctx, containerID, cmd)
	return err
}

// ChmodContainerPath 修改容器文件权限
func (c *Client) ChmodContainerPath(ctx context.Context, containerID, rawPath, mode string, recursive bool) error {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return err
	}
	if safePath == "/" {
		return fmt.Errorf("refusing to chmod root directory /")
	}

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

	_, err = c.ExecContainer(ctx, containerID, cmd)
	return err
}
