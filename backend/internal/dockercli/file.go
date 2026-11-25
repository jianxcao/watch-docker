package dockercli

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
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
// 仅用 TrimSpace 判定空输入；对非空输入保留原始字符（包括前后空格）传给 path.Clean，避免 "report " 与 "report" 混淆。
func cleanContainerPath(rawPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(rawPath, 0) {
		return "", fmt.Errorf("invalid path: null byte")
	}
	cleaned := path.Clean("/" + rawPath)
	return cleaned, nil
}

var errExecOutputLimit = errors.New("exec output exceeds limit")

type limitedWriter struct {
	buf      *bytes.Buffer
	limit    int64
	exceeded bool
	onLimit  func()
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.limit > 0 && int64(w.buf.Len())+int64(len(p)) > w.limit {
		w.exceeded = true
		if w.onLimit != nil {
			w.onLimit()
		}
		return 0, errExecOutputLimit
	}
	return w.buf.Write(p)
}

// ExecContainer 在容器中执行命令并返回 stdout 输出（限制最大缓冲 16MB 防止异常输出触发 OOM）
func (c *Client) ExecContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	return c.execContainerWithEnvAndLimit(ctx, containerID, cmd, nil, 16*1024*1024)
}

func (c *Client) execContainerWithLimit(ctx context.Context, containerID string, cmd []string, maxStdoutBytes int64) (string, error) {
	return c.execContainerWithEnvAndLimit(ctx, containerID, cmd, nil, maxStdoutBytes)
}

func (c *Client) execContainerWithEnvAndLimit(ctx context.Context, containerID string, cmd, env []string, maxStdoutBytes int64) (string, error) {
	execConfig := client.ExecCreateOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
		Cmd:          cmd,
		Env:          env,
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

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutW := &limitedWriter{
		buf:     &stdoutBuf,
		limit:   maxStdoutBytes,
		onLimit: attachResp.Close,
	}
	stderrW := &limitedWriter{
		buf:     &stderrBuf,
		limit:   1024 * 1024,
		onLimit: attachResp.Close,
	}
	if _, err := stdcopy.StdCopy(stdoutW, stderrW, attachResp.Reader); err != nil {
		if stdoutW.exceeded || errors.Is(err, errExecOutputLimit) {
			return "", errExecOutputLimit
		}
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
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr == "" {
			stderrStr = fmt.Sprintf("exit code %d", inspectResp.ExitCode)
		}
		return "", fmt.Errorf("command exited with code %d: %s", inspectResp.ExitCode, stderrStr)
	}

	if stderrBuf.Len() > 0 {
		logger.Logger.Debug("exec command stderr",
			zap.String("container", containerID),
			zap.Strings("cmd", cmd),
			zap.String("stderr", stderrBuf.String()))
	}

	return stdoutBuf.String(), nil
}

var (
	lsRegularLineRe  = regexp.MustCompile(`^([-dlsp][rwxstST-]{9}[.+]?)\s+\d+\s+(\S+)\s+(\S+)\s+(\d+)\s+(.+)$`)
	lsDeviceLineRe   = regexp.MustCompile(`^([cb][rwxstST-]{9}[.+]?)\s+\d+\s+(\S+)\s+(\S+)\s+(\d+,\s*\d+)\s+(.+)$`)
	lsFullTimeRe     = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?\s+[+-]\d{4}) (.+)$`)
	lsCustomTimeRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}) (.+)$`)
	lsIsoTimeRe      = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}) (.+)$`)
	lsIsoShortTimeRe = regexp.MustCompile(`^(\d{2}-\d{2}\s+\d{2}:\d{2}) (.+)$`)
	lsIsoDateOnlyRe  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}) (?: (.+)|([^ ].*))$`)
	lsTradTimeRe     = regexp.MustCompile(`^([A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}(?::\d{2})?) (.+)$`)
	lsYearTimeRe     = regexp.MustCompile(`^([A-Za-z]{3}\s+\d{1,2}\s+\d{4}) (.+)$`)
)

// parseLsOutput 解析 ls -la 命令输出，逐字保留条目名和链接目标中的前后空格。
// 遇到无法明确判定日期边界或符号链接分隔符有歧义的行时返回错误，触发上层回退到 Docker Archive API。
func parseLsOutput(output string) ([]FileEntry, error) {
	return parseLsOutputAt(output, time.Now().UTC())
}

func parseLsOutputAt(output string, now time.Time) ([]FileEntry, error) {
	lines := strings.Split(output, "\n")
	entries := []FileEntry{}
	refNow := now.UTC()

	timeRegexes := []*regexp.Regexp{
		lsFullTimeRe,
		lsCustomTimeRe,
		lsIsoTimeRe,
		lsIsoShortTimeRe,
		lsIsoDateOnlyRe,
		lsTradTimeRe,
		lsYearTimeRe,
	}

	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || line == "total" || strings.HasPrefix(line, "total ") {
			continue
		}

		isDevice := false
		matches := lsRegularLineRe.FindStringSubmatch(line)
		if len(matches) < 6 {
			matches = lsDeviceLineRe.FindStringSubmatch(line)
			if len(matches) < 6 {
				return nil, fmt.Errorf("unrecognized ls output line: %q", line)
			}
			isDevice = true
		}

		perms := matches[1]
		owner := matches[2]
		group := matches[3]
		sizeStr := matches[4]
		remaining := strings.TrimLeft(matches[5], " \t")

		var dateStr, namePart string
		matched := false
		for _, tre := range timeRegexes {
			if timeMatch := tre.FindStringSubmatch(remaining); len(timeMatch) >= 3 {
				dateStr = timeMatch[1]
				if len(timeMatch) >= 4 && timeMatch[2] == "" {
					namePart = timeMatch[3]
				} else {
					namePart = timeMatch[2]
				}
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("ambiguous ls timestamp format: %q", line)
		}

		fileType := "other"
		var linkTarget string
		name := namePart

		if strings.HasPrefix(perms, "d") {
			fileType = "directory"
		} else if strings.HasPrefix(perms, "l") {
			fileType = "symlink"
			if strings.Count(namePart, " -> ") != 1 {
				return nil, fmt.Errorf("ambiguous symlink entry in ls output: %q", line)
			}
			parts := strings.SplitN(namePart, " -> ", 2)
			name = parts[0]
			linkTarget = parts[1]
		} else if strings.HasPrefix(perms, "-") {
			fileType = "file"
		}

		if name == "." || name == ".." {
			continue
		}

		var size int64
		if !isDevice {
			var err error
			size, err = strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				logger.Logger.Warn("parse size failed", zap.String("size", sizeStr), zap.Error(err))
				size = 0
			}
		}

		readonly := true
		if len(perms) >= 10 {
			if perms[2] == 'w' || perms[5] == 'w' || perms[8] == 'w' {
				readonly = false
			}
		}

		modified, err := parseFileTimeAt(dateStr, refNow)
		if err != nil {
			logger.Logger.Debug("parse time failed, using reference time", zap.String("time", dateStr), zap.Error(err))
			modified = refNow.Format(time.RFC3339)
		}

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
	reMMDDHHMM = regexp.MustCompile(`^(\d{2})-(\d{2}) (\d{2}:\d{2})$`)
	reYYYYMMDD = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
)

// parseFileTime 解析文件时间（使用当前 UTC 时间推断缺省年份）。
func parseFileTime(timeStr string) string {
	now := time.Now().UTC()
	if formatted, err := parseFileTimeAt(timeStr, now); err == nil {
		return formatted
	}
	return now.Format(time.RFC3339)
}

func inferYearlessTime(ref time.Time, parseForYear func(year int) (time.Time, error)) (time.Time, error) {
	ref = ref.UTC()
	refYear := ref.Year()
	if t, err := parseForYear(refYear); err == nil {
		if !t.After(ref.Add(time.Hour)) {
			return t, nil
		}
		prev, perr := parseForYear(refYear - 1)
		if perr != nil {
			return time.Time{}, perr
		}
		return prev, nil
	}
	// 仅当当前年无该日历日（如非闰年中的 Feb 29）时才尝试上一年，防止将去年同日误丢弃
	prev, err := parseForYear(refYear - 1)
	if err != nil {
		return time.Time{}, err
	}
	if prev.After(ref.Add(time.Hour)) {
		return time.Time{}, fmt.Errorf("inferred previous-year time %s is after reference %s", prev, ref)
	}
	return prev, nil
}

// parseFileTimeAt 按给定参考时刻解析 ls 时间戳并统一返回 RFC3339（UTC）。
func parseFileTimeAt(timeStr string, now time.Time) (string, error) {
	timeStr = strings.TrimSpace(timeStr)
	timeStr = reSpaces.ReplaceAllString(timeStr, " ")
	timeStr = reNano.ReplaceAllString(timeStr, "${1} ")
	ref := now.UTC()

	if t, err := time.Parse("2006-01-02 15:04:05 -0700", timeStr); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}

	explicitUTCLayouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"Jan 2 2006",
	}
	for _, layout := range explicitUTCLayouts {
		if t, err := time.ParseInLocation(layout, timeStr, time.UTC); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}

	if matched := reYYYYMMDD.FindStringSubmatch(timeStr); len(matched) == 4 {
		dateStr := fmt.Sprintf("%s-%s-%s 00:00", matched[1], matched[2], matched[3])
		if t, err := time.ParseInLocation("2006-01-02 15:04", dateStr, time.UTC); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
		return "", fmt.Errorf("invalid YYYY-MM-DD date %q", timeStr)
	}

	for _, pair := range []struct {
		checkLayout string
		withYear    string
	}{
		{"Jan 2 15:04:05", "2006 Jan 2 15:04:05"},
		{"Jan 2 15:04", "2006 Jan 2 15:04"},
	} {
		// 仅筛选格式结构匹配无年份英文月日的输入；用 2004（闰年）避免先验过滤 Feb 29
		if _, err := time.Parse(pair.checkLayout, timeStr); err == nil || strings.HasPrefix(strings.ToLower(timeStr), "feb 29 ") {
			if _, febCheckErr := time.Parse(pair.withYear, "2004 "+timeStr); febCheckErr == nil {
				t, ierr := inferYearlessTime(ref, func(year int) (time.Time, error) {
					return time.ParseInLocation(pair.withYear, fmt.Sprintf("%04d %s", year, timeStr), time.UTC)
				})
				if ierr != nil {
					return "", ierr
				}
				return t.UTC().Format(time.RFC3339), nil
			}
		}
	}

	if matched := reMMDDHHMM.FindStringSubmatch(timeStr); len(matched) == 4 {
		t, err := inferYearlessTime(ref, func(year int) (time.Time, error) {
			dateStr := fmt.Sprintf("%04d-%s-%s %s", year, matched[1], matched[2], matched[3])
			return time.ParseInLocation("2006-01-02 15:04", dateStr, time.UTC)
		})
		if err != nil {
			return "", err
		}
		return t.UTC().Format(time.RFC3339), nil
	}

	if matched := reMMDD.FindStringSubmatch(timeStr); len(matched) == 3 {
		t, err := inferYearlessTime(ref, func(year int) (time.Time, error) {
			dateStr := fmt.Sprintf("%04d-%s-%s 00:00", year, matched[1], matched[2])
			return time.ParseInLocation("2006-01-02 15:04", dateStr, time.UTC)
		})
		if err != nil {
			return "", err
		}
		return t.UTC().Format(time.RFC3339), nil
	}

	return "", fmt.Errorf("unrecognized file timestamp: %q", timeStr)
}

// ListContainerDirectory 列出容器目录内容
// 优先使用 exec ls 命令，全部失败时降级到 Docker Archive API
func (c *Client) ListContainerDirectory(ctx context.Context, containerID, rawPath string) (*FileListResult, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return nil, err
	}

	lsTarget := safePath
	if lsTarget != "/" {
		lsTarget += "/"
	}

	commands := [][]string{
		{"ls", "-la", "--time-style=+%Y-%m-%d %H:%M:%S", lsTarget},
		{"ls", "-la", "--time-style=iso", lsTarget},
		{"ls", "-la", "--full-time", lsTarget},
		{"ls", "-la", lsTarget},
	}
	lsEnv := []string{"TZ=UTC0", "LC_ALL=C"}

	var output string
	var lastErr error

	for i, cmd := range commands {
		output, lastErr = c.execContainerWithEnvAndLimit(ctx, containerID, cmd, lsEnv, 16*1024*1024)
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

	entries, err := parseLsOutputAt(output, time.Now().UTC())
	if err != nil {
		logger.Logger.Info("ls output ambiguous, falling back to archive API",
			zap.String("container", containerID),
			zap.String("path", safePath),
			zap.Error(err))
		return c.listContainerDirectoryFromArchive(ctx, containerID, safePath)
	}

	return &FileListResult{
		Path:    safePath,
		Entries: entries,
	}, nil
}

// listContainerDirectoryFromArchive 通过 Docker Archive API (CopyFromContainer) 列出目录
func (c *Client) listContainerDirectoryFromArchive(ctx context.Context, containerID, safePath string) (*FileListResult, error) {
	archivePath := safePath
	if archivePath != "/" {
		archivePath += "/"
	}
	copyRes, err := c.docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: archivePath})
	if err != nil {
		return nil, fmt.Errorf("archive API failed: %w", err)
	}
	defer copyRes.Content.Close()

	if copyRes.Stat.Mode != 0 && !copyRes.Stat.Mode.IsDir() && copyRes.Stat.Mode&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("path %s is not a directory", safePath)
	}

	prefixes := []string{}
	if safePath != "/" {
		basePrefix := path.Base(safePath) + "/"
		prefixes = append(prefixes, basePrefix)
		statName := strings.Trim(strings.TrimPrefix(copyRes.Stat.Name, "./"), "/")
		if statName != "" && statName != "." && statName != ".." {
			statPrefix := statName + "/"
			if statPrefix != basePrefix {
				prefixes = append(prefixes, statPrefix)
			}
		}
	}

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

		name := strings.Trim(strings.TrimPrefix(header.Name, "./"), "/")
		if name == "" || name == "." || name == ".." {
			continue
		}

		var childName string
		if safePath == "/" {
			if strings.Contains(name, "/") {
				continue
			}
			childName = name
		} else {
			matchedPrefix := false
			for _, prefix := range prefixes {
				if strings.HasPrefix(name, prefix) {
					rel := strings.TrimPrefix(name, prefix)
					if rel != "" && rel != "." && rel != ".." && !strings.Contains(rel, "/") {
						childName = rel
						matchedPrefix = true
					}
					break
				}
			}
			if !matchedPrefix {
				continue
			}
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

var (
	errArchiveSymlink       = errors.New("archive entry is a symlink")
	errNonRegularFile       = errors.New("path is not a regular file")
	errEditableFileTooLarge = errors.New("file exceeds online editable limit")
)

// ReadContainerFile 读取容器文件内容
func (c *Client) ReadContainerFile(ctx context.Context, containerID, rawPath string) (string, error) {
	safePath, err := cleanContainerPath(rawPath)
	if err != nil {
		return "", err
	}

	cmd := []string{
		"sh", "-c",
		`if [ -p "$1" ] || [ -d "$1" ] || [ -b "$1" ] || [ -c "$1" ] || [ -S "$1" ]; then echo "path is not a regular file" >&2; exit 1; fi; exec cat -- "$1"`,
		"sh", safePath,
	}
	output, err := c.execContainerWithLimit(ctx, containerID, cmd, MaxEditableFileSize)
	if err != nil {
		if errors.Is(err, errExecOutputLimit) {
			return "", fmt.Errorf("文件过大（超过 %d MB 在线编辑限制），请使用下载功能", MaxEditableFileSize/(1024*1024))
		}
		return "", fmt.Errorf("failed to read file: %w", err)
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
		return "", fmt.Errorf("%w: path is a directory, not a file", errNonRegularFile)
	}
	if header.Typeflag == tar.TypeSymlink {
		return "", fmt.Errorf("%w: %s", errArchiveSymlink, safePath)
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return "", fmt.Errorf("%w (type=%c)", errNonRegularFile, header.Typeflag)
	}
	if header.Size > MaxEditableFileSize {
		return "", fmt.Errorf("%w: 文件过大（超过 %d MB 在线编辑限制），请使用下载功能", errEditableFileTooLarge, MaxEditableFileSize/(1024*1024))
	}

	limited := io.LimitReader(tarReader, MaxEditableFileSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}
	if int64(len(content)) > MaxEditableFileSize {
		return "", fmt.Errorf("%w: 文件过大（超过 %d MB 在线编辑限制），请使用下载功能", errEditableFileTooLarge, MaxEditableFileSize/(1024*1024))
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

	// 若已确认是非普通文件（FIFO/目录/设备等）或超过大小限制，直接返回错误，避免降级 cat 阻塞或 OOM
	if errors.Is(err, errNonRegularFile) || errors.Is(err, errEditableFileTooLarge) {
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
	if filename == "" || filename == "." || filename == ".." ||
		strings.ContainsAny(filename, "/\\") || strings.ContainsRune(filename, 0) {
		return fmt.Errorf("invalid filename")
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		tw := tar.NewWriter(pw)
		hdr := &tar.Header{
			Name:    filename,
			Mode:    0644,
			Size:    size,
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = pw.CloseWithError(err)
			errCh <- err
			return
		}
		if _, err := io.CopyN(tw, src, size); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
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
