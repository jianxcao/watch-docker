package composecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

type Client struct {
	docker                *client.Client
	RuntimeProjectsLister func(ctx context.Context) ([]ListOutput, error)
}

func NewClient(docker *client.Client) *Client {
	return &Client{
		docker: docker,
	}
}

var (
	validProjectNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	// ErrSymlinkPath 表示路径自身或其 Compose 文件为符号链接
	ErrSymlinkPath = errors.New("拒绝访问符号链接路径")
	// ErrTraversalPath 表示路径包含非法目录穿越片段
	ErrTraversalPath = errors.New("非法的目录穿越路径")
)

// ValidateProjectName 严格校验 Compose 项目名称，禁止路径分隔符、上级引用或以 - 开头
func ValidateProjectName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("项目名称不能为空")
	}
	if trimmed == "." || trimmed == ".." || strings.Contains(trimmed, "..") {
		return errors.New("非法的项目名称")
	}
	if strings.ContainsAny(trimmed, "/\\\x00") {
		return errors.New("项目名称不能包含路径分隔符")
	}
	if !validProjectNameRe.MatchString(trimmed) {
		return errors.New("项目名称只能包含字母、数字、点、下划线和连字符，且必须以字母或数字开头")
	}
	return nil
}

func resolveAppRoot() (string, error) {
	appPath := strings.TrimSpace(conf.EnvCfg.APP_PATH)
	if appPath == "" {
		return "", errors.New("APP_PATH 未设置，禁止操作外部目录")
	}
	absApp, err := filepath.Abs(appPath)
	if err != nil {
		return "", fmt.Errorf("解析 APP_PATH 失败: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absApp); err == nil {
		absApp = resolved
	}
	return filepath.Clean(absApp), nil
}

func resolvePathCheckingSymlinks(target string) (string, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absTarget); err == nil {
		return filepath.Clean(resolved), nil
	}
	// 目标尚不存在时解析其父目录符号链接
	parent := filepath.Dir(absTarget)
	base := filepath.Base(absTarget)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Clean(filepath.Join(resolvedParent, base)), nil
	}
	return filepath.Clean(absTarget), nil
}

func ensureSubdirOfAppRoot(dir string) (string, error) {
	appRoot, err := resolveAppRoot()
	if err != nil {
		return "", err
	}
	resolvedDir, err := resolvePathCheckingSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("解析项目目录失败: %w", err)
	}
	rel, err := filepath.Rel(appRoot, resolvedDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) || strings.ContainsRune(rel, filepath.Separator) {
		return "", fmt.Errorf("拒绝访问 APP_PATH 之外或非一级子目录路径: %s", dir)
	}
	return resolvedDir, nil
}

// ValidateProjectDir 校验项目目录必须严格位于 APP_PATH 一级子目录内，且目录本身不为符号链接
func ValidateProjectDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("项目目录不能为空")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("解析项目目录失败: %w", err)
	}
	if fi, lerr := os.Lstat(absDir); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, absDir)
	} else if lerr != nil && !os.IsNotExist(lerr) {
		return "", fmt.Errorf("检查项目目录失败: %w", lerr)
	}
	return ensureSubdirOfAppRoot(absDir)
}

func hasPathTraversalSegment(rawPath string) bool {
	slashed := filepath.ToSlash(strings.TrimSpace(rawPath))
	for _, seg := range strings.Split(slashed, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// ValidateComposeFilePath 校验 composeFile 必须位于 APP_PATH 一级子目录内，文件名为合法的 compose 文件且非符号链接
func ValidateComposeFilePath(composeFile string) (projectDir string, cleanFile string, err error) {
	trimmed := strings.TrimSpace(composeFile)
	if trimmed == "" {
		return "", "", errors.New("composeFile 不能为空")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", "", errors.New("非法的 composeFile 路径")
	}
	if hasPathTraversalSegment(trimmed) {
		return "", "", fmt.Errorf("%w: %s", ErrTraversalPath, composeFile)
	}
	// docker compose ls 可能返回逗号分隔的多个路径，只允许操作受控单文件
	if strings.Contains(trimmed, ",") {
		return "", "", errors.New("不支持同时指定多个 composeFile")
	}
	absFile, err := filepath.Abs(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("解析 composeFile 路径失败: %w", err)
	}
	base := filepath.Base(absFile)
	if !isComposeFileName(base) {
		return "", "", fmt.Errorf("非法 compose 文件名: %s", base)
	}
	if fi, lerr := os.Lstat(absFile); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("%w: %s", ErrSymlinkPath, absFile)
	} else if lerr != nil && !os.IsNotExist(lerr) {
		return "", "", fmt.Errorf("检查 composeFile 失败: %w", lerr)
	}
	rawDir := filepath.Dir(absFile)
	if fi, lerr := os.Lstat(rawDir); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("%w: %s", ErrSymlinkPath, rawDir)
	} else if lerr != nil && !os.IsNotExist(lerr) {
		return "", "", fmt.Errorf("检查项目目录失败: %w", lerr)
	}
	validDir, err := ensureSubdirOfAppRoot(rawDir)
	if err != nil {
		return "", "", err
	}
	cleanFile = filepath.Join(validDir, base)
	if fi, lerr := os.Lstat(cleanFile); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
	} else if lerr != nil && !os.IsNotExist(lerr) {
		return "", "", fmt.Errorf("检查 composeFile 失败: %w", lerr)
	}
	return validDir, cleanFile, nil
}

// ScanProjects 扫描发现 Compose 项目
func (c *Client) ScanProjects(ctx context.Context) []ComposeProject {
	var projects []ComposeProject
	appPath, err := resolveAppRoot()
	if err != nil {
		return projects
	}

	const maxDepth = 2

	err = filepath.Walk(appPath, func(curPath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Logger.Error("扫描项目失败", logger.ZapErr(err))
			return nil
		}

		relPath, err := filepath.Rel(appPath, curPath)
		if err != nil {
			return nil
		}

		depth := 0
		if relPath != "." {
			depth = len(strings.Split(relPath, string(os.PathSeparator)))
		}

		if info.IsDir() && depth >= maxDepth {
			return filepath.SkipDir
		}

		if !info.IsDir() && info.Mode().IsRegular() && isPrimaryComposeFileName(info.Name()) && depth == maxDepth {
			projectName := filepath.Base(filepath.Dir(curPath))
			logger.Logger.Debug("扫描到项目", zap.String("curPath", curPath), zap.String("name", projectName))
			project := ComposeProject{
				Name:         projectName,
				ComposeFile:  curPath,
				Status:       StatusDraft,
				RunningCount: 0,
				ExitedCount:  0,
				CreatedCount: 0,
			}
			projects = append(projects, project)
		}
		return nil
	})
	if err != nil {
		logger.Logger.Error("扫描项目失败", logger.ZapErr(err))
		return projects
	}

	return projects
}

var (
	primaryComposeFiles  = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}
	overrideComposeFiles = []string{"compose.override.yaml", "compose.override.yml", "docker-compose.override.yaml", "docker-compose.override.yml"}
)

func isPrimaryComposeFileName(filename string) bool {
	switch filename {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	default:
		return false
	}
}

func isComposeFileName(filename string) bool {
	if isPrimaryComposeFileName(filename) {
		return true
	}
	switch filename {
	case "docker-compose.override.yml", "docker-compose.override.yaml", "compose.override.yml", "compose.override.yaml":
		return true
	default:
		return false
	}
}

func (c *Client) isComposeFile(filename string) bool {
	return isComposeFileName(filename)
}

func (c *Client) findComposeFileInDir(dir string) string {
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, cf := range composeFiles {
		fullPath := filepath.Join(dir, cf)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	return filepath.Join(dir, "docker-compose.yaml")
}

func parseStatusInfo(oStatus string) (status StackStatus, runningCount int, exitedCount int, createdCount int) {
	if oStatus == "" {
		return "unknown", 0, 0, 0
	}

	re := regexp.MustCompile(`(\w+)(?:\((\d+)\))?`)
	matches := re.FindAllStringSubmatch(oStatus, -1)

	for _, match := range matches {
		statusName := match[1]
		count := 1

		if len(match) > 2 && match[2] != "" {
			if c, err := strconv.Atoi(match[2]); err == nil {
				count = c
			}
		}
		switch statusName {
		case "running":
			runningCount = count
		case "exited":
			exitedCount = count
		case "created":
			createdCount = count
		}
	}

	if strings.HasPrefix(oStatus, "created") {
		status = StatusCreatedStack
	} else if runningCount > 0 && exitedCount > 0 {
		status = StatusPartial
	} else if runningCount > 0 {
		status = StatusRunning
	} else if exitedCount > 0 {
		status = StatusExited
	} else {
		status = StatusUnknown
	}

	return status, runningCount, exitedCount, createdCount
}

type ListOutput struct {
	Status      string `json:"status"`
	Name        string `json:"name"`
	ConfigFiles string `json:"ConfigFiles"`
}

func (c *Client) listRuntimeProjects(ctx context.Context) ([]ListOutput, error) {
	if c != nil && c.RuntimeProjectsLister != nil {
		return c.RuntimeProjectsLister(ctx)
	}
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      "",
		Args:          []string{"ls", "-a", "--format", "json"},
		OperationName: "list projects",
		NeedOutput:    true,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	output := strings.TrimSpace(string(res.Output))
	var listOutput []ListOutput
	if output != "" {
		if err := json.Unmarshal([]byte(output), &listOutput); err != nil {
			logger.Logger.Error("解析项目列表失败", zap.String("output", output), logger.ZapErr(err))
			return nil, err
		}
	}
	return listOutput, nil
}

// FindRuntimeProject 检查指定项目名是否存在于 docker compose 运行时列表中
func (c *Client) FindRuntimeProject(ctx context.Context, projectName string) (string, bool, error) {
	if err := ValidateProjectName(projectName); err != nil {
		return "", false, err
	}
	runtimeList, err := c.listRuntimeProjects(ctx)
	if err != nil {
		return "", false, err
	}
	for _, item := range runtimeList {
		if strings.EqualFold(item.Name, strings.TrimSpace(projectName)) {
			if err := ValidateProjectName(item.Name); err != nil {
				continue
			}
			return item.Name, true, nil
		}
	}
	return "", false, nil
}

// ResolveComposeTarget 解析 Compose 生命周期执行目标：
// 1. 优先校验 composeFile 位于 APP_PATH 内且非符号链接，直接返回 (projectDir, nil)；
// 2. 若路径包含 .. 穿越直接拒绝；若路径位于 APP_PATH 内但为符号链接或非法文件名也直接拒绝，不回退到 -p 模式；
// 3. 若 APP_PATH 未设置或 composeFile 为空/位于 APP_PATH 之外（包括使用自定义 -f 文件名的外部运行时栈），验证 projectName 存在于 docker compose 运行时列表后，
//    返回 ("", []string{"-p", runtimeName}) 以支持外部/运行时 Compose 栈的启停、日志与下线。
func isPathWithinAppRoot(rawPath string) bool {
	appPath := strings.TrimSpace(conf.EnvCfg.APP_PATH)
	if appPath == "" || strings.TrimSpace(rawPath) == "" {
		return false
	}
	absTarget, err := filepath.Abs(strings.TrimSpace(rawPath))
	if err != nil {
		return false
	}
	absApp, err := filepath.Abs(appPath)
	if err != nil {
		return false
	}
	isSub := func(base, target string) bool {
		rel, rerr := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
		return rerr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	if isSub(absApp, absTarget) {
		return true
	}
	if resolvedApp, rerr := resolveAppRoot(); rerr == nil {
		if resolvedTarget, terr := resolvePathCheckingSymlinks(absTarget); terr == nil && isSub(resolvedApp, resolvedTarget) {
			return true
		}
		if isSub(resolvedApp, absTarget) {
			return true
		}
	}
	return false
}

func (c *Client) ResolveComposeTarget(ctx context.Context, projectName, composeFile string) (string, []string, error) {
	trimmedFile := strings.TrimSpace(composeFile)
	if trimmedFile != "" {
		projectPath, cleanFile, err := ValidateComposeFilePath(trimmedFile)
		if err == nil {
			if fi, lerr := os.Lstat(cleanFile); lerr == nil && fi.Mode().IsRegular() {
				return projectPath, nil, nil
			} else if lerr != nil && !os.IsNotExist(lerr) {
				return "", nil, fmt.Errorf("检查 composeFile 失败: %w", lerr)
			}
		} else {
			if errors.Is(err, ErrTraversalPath) ||
				(isPathWithinAppRoot(trimmedFile) && (errors.Is(err, ErrSymlinkPath) || strings.HasPrefix(err.Error(), "非法 compose 文件名"))) {
				return "", nil, err
			}
		}
	}

	if strings.TrimSpace(projectName) != "" {
		runtimeName, found, err := c.FindRuntimeProject(ctx, projectName)
		if err == nil && found {
			return "", []string{"-p", runtimeName}, nil
		}
	}

	if trimmedFile != "" {
		_, _, err := ValidateComposeFilePath(trimmedFile)
		if err != nil {
			return "", nil, err
		}
		return "", nil, errors.New("compose 文件不存在")
	}
	return "", nil, errors.New("项目不存在或未指定合法的 composeFile")
}

func (c *Client) ListProjects(ctx context.Context) ([]ComposeProject, error) {
	projects := c.ScanProjects(ctx)
	listOutput, err := c.listRuntimeProjects(ctx)
	if err != nil {
		return projects, err
	}
	result := mergeComposeProjects(projects, listOutput)
	return result, nil
}

func mergeComposeProjects(scanned []ComposeProject, runtime []ListOutput) []ComposeProject {
	result := append([]ComposeProject(nil), scanned...)
	pathIndexes := make(map[string]int, len(scanned))
	nameIndexes := make(map[string][]int, len(scanned))
	for i, project := range scanned {
		if normalizedPath := normalizeComposePath(project.ComposeFile); normalizedPath != "" {
			pathIndexes[normalizedPath] = i
		}
		nameKey := strings.ToLower(project.Name)
		nameIndexes[nameKey] = append(nameIndexes[nameKey], i)
	}

	for _, item := range runtime {
		project := composeProjectFromListOutput(item)
		matchIndex := -1
		for _, configFile := range strings.Split(item.ConfigFiles, ",") {
			if index, ok := pathIndexes[normalizeComposePath(configFile)]; ok {
				matchIndex = index
				break
			}
		}
		if matchIndex == -1 {
			indexes := nameIndexes[strings.ToLower(item.Name)]
			if len(indexes) == 1 {
				matchIndex = indexes[0]
			}
		}

		if matchIndex == -1 {
			result = append(result, project)
			continue
		}

		result[matchIndex].Status = project.Status
		result[matchIndex].RunningCount = project.RunningCount
		result[matchIndex].ExitedCount = project.ExitedCount
		result[matchIndex].CreatedCount = project.CreatedCount
	}

	return result
}

func normalizeComposePath(composeFile string) string {
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return ""
	}
	absPath, err := filepath.Abs(composeFile)
	if err != nil {
		return filepath.Clean(composeFile)
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(absPath)
}

func composeProjectFromListOutput(item ListOutput) ComposeProject {
	status, runningCount, exitedCount, createdCount := parseStatusInfo(item.Status)
	return ComposeProject{
		Name:         item.Name,
		ComposeFile:  item.ConfigFiles,
		Status:       status,
		RunningCount: runningCount,
		ExitedCount:  exitedCount,
		CreatedCount: createdCount,
	}
}

func (c *Client) runProjectCommand(ctx context.Context, projectName, composeFile, opName, logLabel string, opArgs []string) error {
	projectPath, argsPrefix, err := c.ResolveComposeTarget(ctx, projectName, composeFile)
	if err != nil {
		return err
	}
	args := append(append([]string(nil), argsPrefix...), opArgs...)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          args,
		OperationName: opName,
		NeedOutput:    true,
	})
	logger.Logger.Info(logLabel, zap.String("output", string(res.Output)))
	return res.Error
}

// StartProject 启动 Compose 项目（APP_PATH 内基于目录，外部运行时项目基于 -p <name>）
func (c *Client) StartProject(ctx context.Context, projectName, composeFile string) error {
	return c.runProjectCommand(ctx, projectName, composeFile, "start project", "启动APP", []string{"start"})
}

// StopProject 停止项目中的所有服务
func (c *Client) StopProject(ctx context.Context, projectName, composeFile string) error {
	return c.runProjectCommand(ctx, projectName, composeFile, "stop project", "停止APP", []string{"stop"})
}

// RestartProject 重启项目
func (c *Client) RestartProject(ctx context.Context, projectName, composeFile string) error {
	return c.runProjectCommand(ctx, projectName, composeFile, "restart project", "重启APP", []string{"restart"})
}

// PullProject 拉取项目镜像
func (c *Client) PullProject(ctx context.Context, composeFile string) error {
	projectPath, cleanFile, err := ValidateComposeFilePath(composeFile)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(cleanFile); err != nil {
		return errors.New("compose 文件不存在: " + err.Error())
	}
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"pull"},
		OperationName: "pull project",
		NeedOutput:    true,
	})
	logger.Logger.Info("拉取镜像", zap.String("output", string(res.Output)))
	return res.Error
}

// DeleteProject 删除项目及其所有资源（StatusDraft 目录删除严格要求位于 APP_PATH 内且 compose 文件为常规文件）
func (c *Client) DeleteProject(ctx context.Context, projectName, composeFile string, status StackStatus) error {
	if status != StatusDraft {
		return c.runProjectCommand(ctx, projectName, composeFile, "delete project", "删除APP（Docker资源）", []string{"down", "--volumes", "--remove-orphans"})
	}

	projectPath, cleanFile, err := ValidateComposeFilePath(composeFile)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(cleanFile)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("compose 文件不存在")
		}
		return errors.New("检查 compose 文件失败: " + err.Error())
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
	}
	dirName := filepath.Base(projectPath)

	projects, err := c.ListProjects(ctx)
	if err != nil {
		logger.Logger.Error("查询项目状态失败", zap.String("project", dirName), logger.ZapErr(err))
		return errors.New("查询项目状态失败: " + err.Error())
	}

	currentStatus := StatusDraft
	normalizedClean := normalizeComposePath(cleanFile)
	for _, p := range projects {
		if strings.EqualFold(p.Name, dirName) || (normalizedClean != "" && normalizeComposePath(p.ComposeFile) == normalizedClean) {
			currentStatus = p.Status
			break
		}
	}

	if currentStatus != StatusDraft {
		res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
			ExecPath:      projectPath,
			Args:          []string{"down", "--volumes", "--remove-orphans"},
			OperationName: "delete project (status changed)",
			NeedOutput:    true,
		})
		if res.Error != nil {
			return res.Error
		}
		return nil
	}

	logger.Logger.Info("删除项目目录", zap.String("path", projectPath), zap.String("status", string(currentStatus)))
	if err := os.RemoveAll(projectPath); err != nil {
		logger.Logger.Error("删除项目目录失败", zap.String("path", projectPath), logger.ZapErr(err))
		return errors.New("删除项目目录失败: " + err.Error())
	}

	return nil
}

func (c *Client) CreateProject(ctx context.Context, composeFile string, isRuning bool, isBuild bool) error {
	projectPath, cleanFile, err := ValidateComposeFilePath(composeFile)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(cleanFile); err != nil {
		return errors.New("compose 文件不存在: " + err.Error())
	}
	args := []string{"up", "-d", "--remove-orphans"}
	if isRuning {
		args = append(args, "--force-recreate")
	}
	if isBuild {
		args = append(args, "--build")
	}
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          args,
		OperationName: "create project",
		NeedOutput:    true,
	})
	logger.Logger.Info("创建APP", zap.String("output", string(res.Output)))
	return res.Error
}

// SaveNewProject 保存新的 Compose 项目（严格校验项目名称及位于 APP_PATH 内，拒绝符号链接）
func (c *Client) SaveNewProject(ctx context.Context, name string, yamlContent string, force bool) (string, error) {
	return c.SaveProject(ctx, name, "", yamlContent, force)
}

// SaveProject 保存或更新指定 Compose 项目的配置文件，并返回实际写入的文件路径。
func (c *Client) SaveProject(ctx context.Context, name string, composeFile string, yamlContent string, force bool) (string, error) {
	savedFile, _, err := c.SaveProjectForDeploy(ctx, name, composeFile, yamlContent, force)
	return savedFile, err
}

func validateProjectMemberConfigFile(projectPath, projectName, rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" || strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("非法的项目配置文件路径: %s", rawPath)
	}
	if hasPathTraversalSegment(trimmed) {
		return "", fmt.Errorf("%w: %s", ErrTraversalPath, rawPath)
	}
	absFile, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("解析项目配置文件路径 %s 失败: %w", rawPath, err)
	}
	base := filepath.Base(absFile)
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".yml" && ext != ".yaml" {
		return "", fmt.Errorf("非法的 Compose 配置文件 %s", rawPath)
	}
	if fi, lerr := os.Lstat(absFile); lerr != nil {
		if os.IsNotExist(lerr) {
			return "", fmt.Errorf("项目 %s 的配置文件不存在: %s", projectName, rawPath)
		}
		return "", fmt.Errorf("检查配置文件 %s 失败: %w", rawPath, lerr)
	} else if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, absFile)
	}
	rawDir := filepath.Dir(absFile)
	if fi, lerr := os.Lstat(rawDir); lerr != nil {
		return "", fmt.Errorf("检查配置文件目录 %s 失败: %w", rawDir, lerr)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, rawDir)
	}
	validDir, err := ensureSubdirOfAppRoot(rawDir)
	if err != nil || validDir != projectPath {
		return "", fmt.Errorf("运行时配置文件 %s 不属于项目 %s (期望目录: %s)", rawPath, projectName, projectPath)
	}
	cleanFile := filepath.Join(validDir, base)
	if fi, lerr := os.Lstat(cleanFile); lerr != nil {
		return "", fmt.Errorf("检查配置文件 %s 失败: %w", cleanFile, lerr)
	} else if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
	}
	return cleanFile, nil
}

func collectExistingComposeCandidates(projectPath string, names []string) ([]string, error) {
	var found []string
	for _, cf := range names {
		candidate := filepath.Join(projectPath, cf)
		fi, err := os.Lstat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.New("检查 Compose 文件失败: " + err.Error())
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s", ErrSymlinkPath, candidate)
		}
		found = append(found, candidate)
	}
	return found, nil
}

func (c *Client) lookupRuntimeConfigFiles(ctx context.Context, name, projectPath string) (string, bool, error) {
	runtimeList, err := c.listRuntimeProjects(ctx)
	if err != nil {
		if c != nil && c.RuntimeProjectsLister != nil {
			return "", false, err
		}
		logger.Logger.Debug("获取运行时 Compose 项目列表失败，回退目录发现",
			zap.String("project", name),
			logger.ZapErr(err))
		return "", false, nil
	}
	trimmedName := strings.TrimSpace(name)
	for _, item := range runtimeList {
		if strings.EqualFold(strings.TrimSpace(item.Name), trimmedName) && strings.TrimSpace(item.ConfigFiles) != "" {
			return item.ConfigFiles, true, nil
		}
	}
	for _, item := range runtimeList {
		if strings.TrimSpace(item.ConfigFiles) == "" {
			continue
		}
		for _, cfg := range strings.Split(item.ConfigFiles, ",") {
			normalized := normalizeComposePath(cfg)
			if normalized != "" && filepath.Dir(normalized) == projectPath {
				return item.ConfigFiles, true, nil
			}
		}
	}
	return "", false, nil
}

func (c *Client) resolveExistingProjectDeployFiles(ctx context.Context, name, projectPath string) ([]string, error) {
	rawConfigFiles, hasRuntime, err := c.lookupRuntimeConfigFiles(ctx, name, projectPath)
	if err != nil {
		return nil, err
	}
	if hasRuntime {
		var deployFiles []string
		seen := make(map[string]struct{})
		for _, seg := range strings.Split(rawConfigFiles, ",") {
			if strings.TrimSpace(seg) == "" {
				continue
			}
			cleanFile, err := validateProjectMemberConfigFile(projectPath, name, seg)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[cleanFile]; ok {
				continue
			}
			seen[cleanFile] = struct{}{}
			deployFiles = append(deployFiles, cleanFile)
		}
		if len(deployFiles) > 0 {
			return deployFiles, nil
		}
	}

	primaries, err := collectExistingComposeCandidates(projectPath, primaryComposeFiles)
	if err != nil {
		return nil, err
	}
	if len(primaries) == 0 {
		return nil, nil
	}
	overrides, err := collectExistingComposeCandidates(projectPath, overrideComposeFiles)
	if err != nil {
		return nil, err
	}
	deployFiles := []string{primaries[0]}
	if len(overrides) > 0 {
		deployFiles = append(deployFiles, overrides[0])
	}
	return deployFiles, nil
}

// ResolveDeployProjectFiles 解析无需重写 YAML 时的有序部署配置文件列表并校验目标归属。
func (c *Client) ResolveDeployProjectFiles(ctx context.Context, name, composeFile string) (string, []string, error) {
	if err := ValidateProjectName(name); err != nil {
		return "", nil, err
	}
	appRoot, err := resolveAppRoot()
	if err != nil {
		return "", nil, err
	}
	projectPath, err := ValidateProjectDir(filepath.Join(appRoot, name))
	if err != nil {
		return "", nil, err
	}
	stat, err := os.Lstat(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, errors.New("项目目录不存在")
		}
		return "", nil, errors.New("检查项目目录失败: " + err.Error())
	}
	if stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
		return "", nil, fmt.Errorf("%w: %s", ErrSymlinkPath, projectPath)
	}

	deployFiles, err := c.resolveExistingProjectDeployFiles(ctx, name, projectPath)
	if err != nil {
		return "", nil, err
	}
	if trimmedFile := strings.TrimSpace(composeFile); trimmedFile != "" {
		fileProjectDir, cleanFile, err := ValidateComposeFilePath(trimmedFile)
		if err != nil {
			return "", nil, err
		}
		if fileProjectDir != projectPath {
			return "", nil, fmt.Errorf("compose 文件 %s 不属于项目 %s", trimmedFile, name)
		}
		fi, err := os.Lstat(cleanFile)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil, fmt.Errorf("compose 文件不存在: %s", cleanFile)
			}
			return "", nil, errors.New("检查 Compose 文件失败: " + err.Error())
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return "", nil, fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
		}
		if len(deployFiles) > 0 && !slices.Contains(deployFiles, cleanFile) {
			return "", nil, fmt.Errorf("目标配置文件 %s 与项目生效配置文件列表 [%s] 冲突", cleanFile, strings.Join(deployFiles, ", "))
		}
		if len(deployFiles) == 0 {
			deployFiles = []string{cleanFile}
		}
	}
	return projectPath, deployFiles, nil
}

// SaveProjectForDeploy 保存用户打开编辑的单个 Compose 文件，并返回本次写入文件以及部署时需按顺序传入的完整配置文件列表。
// 已有项目优先复用运行时 ConfigFiles（并校验项目归属），否则按 Compose 默认发现主文件及标准覆盖文件；
// 若已有项目的生效文件列表与编辑目标冲突，会在写入前报错并指出具体路径。
func (c *Client) SaveProjectForDeploy(ctx context.Context, name string, composeFile string, yamlContent string, force bool) (string, []string, error) {
	if err := ValidateProjectName(name); err != nil {
		return "", nil, err
	}
	appRoot, err := resolveAppRoot()
	if err != nil {
		return "", nil, err
	}

	projectPath, err := ValidateProjectDir(filepath.Join(appRoot, name))
	if err != nil {
		return "", nil, err
	}

	dirExists := false
	if stat, err := os.Lstat(projectPath); err == nil {
		if stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
			return "", nil, fmt.Errorf("%w: %s", ErrSymlinkPath, projectPath)
		}
		dirExists = true
	} else if !os.IsNotExist(err) {
		return "", nil, errors.New("检查项目目录失败: " + err.Error())
	}

	trimmedFile := strings.TrimSpace(composeFile)
	var targetFile string
	switch {
	case trimmedFile != "":
		fileProjectDir, cleanFile, err := ValidateComposeFilePath(trimmedFile)
		if err != nil {
			return "", nil, err
		}
		if fileProjectDir != projectPath {
			return "", nil, fmt.Errorf("compose 文件 %s 不属于项目 %s", trimmedFile, name)
		}
		if !dirExists {
			return "", nil, errors.New("项目目录不存在")
		}
		if !force {
			logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
			return "", nil, errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		fi, err := os.Lstat(cleanFile)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil, fmt.Errorf("compose 文件不存在: %s", cleanFile)
			}
			return "", nil, errors.New("检查 Compose 文件失败: " + err.Error())
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return "", nil, fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
		}
		targetFile = cleanFile
	case dirExists:
		if !force {
			logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
			return "", nil, errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		primaries, err := collectExistingComposeCandidates(projectPath, primaryComposeFiles)
		if err != nil {
			return "", nil, err
		}
		if len(primaries) > 1 {
			return "", nil, fmt.Errorf("项目目录存在多个 Compose 主配置文件 (%s)，请指定 composeFile", strings.Join(primaries, ", "))
		}
		if len(primaries) == 1 {
			targetFile = primaries[0]
		} else {
			targetFile = filepath.Join(projectPath, "docker-compose.yaml")
		}
	default:
		if rawConfigFiles, hasRuntime, err := c.lookupRuntimeConfigFiles(ctx, name, projectPath); err != nil {
			return "", nil, err
		} else if hasRuntime {
			for _, seg := range strings.Split(rawConfigFiles, ",") {
				if strings.TrimSpace(seg) == "" {
					continue
				}
				if _, err := validateProjectMemberConfigFile(projectPath, name, seg); err != nil {
					return "", nil, err
				}
			}
		}
		targetFile = filepath.Join(projectPath, "docker-compose.yaml")
	}

	var deployFiles []string
	if dirExists {
		resolved, err := c.resolveExistingProjectDeployFiles(ctx, name, projectPath)
		if err != nil {
			return "", nil, err
		}
		if len(resolved) == 0 {
			deployFiles = []string{targetFile}
		} else {
			if !slices.Contains(resolved, targetFile) {
				return "", nil, fmt.Errorf("编辑目标文件 %s 与项目生效配置文件列表 [%s] 冲突", targetFile, strings.Join(resolved, ", "))
			}
			deployFiles = resolved
		}
	} else {
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			logger.Logger.Error("创建项目目录失败", zap.String("path", projectPath), logger.ZapErr(err))
			return "", nil, errors.New("创建项目目录失败: " + err.Error())
		}
		deployFiles = []string{targetFile}
	}

	if fi, err := os.Lstat(targetFile); err == nil && (fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular()) {
		return "", nil, fmt.Errorf("%w: %s", ErrSymlinkPath, targetFile)
	} else if err != nil && !os.IsNotExist(err) {
		return "", nil, errors.New("检查 Compose 文件失败: " + err.Error())
	}

	if err := os.WriteFile(targetFile, []byte(yamlContent), 0644); err != nil {
		logger.Logger.Error("写入 Compose 文件失败", zap.String("file", targetFile), logger.ZapErr(err))
		return "", nil, errors.New("写入 Compose 文件失败: " + err.Error())
	}

	logger.Logger.Info("已保存 Compose 配置文件",
		zap.String("targetFile", targetFile),
		zap.Strings("deployFiles", deployFiles))
	return targetFile, deployFiles, nil
}

// GetProjectYaml 读取项目的 compose YAML 文件内容（严格校验位于 APP_PATH 内且拒绝符号链接）
func (c *Client) GetProjectYaml(file string) (string, error) {
	_, cleanFile, err := ValidateComposeFilePath(file)
	if err != nil {
		return "", err
	}

	fi, err := os.Lstat(cleanFile)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Logger.Error("Compose 文件不存在", zap.String("file", cleanFile))
			return "", errors.New("compose 文件不存在")
		}
		return "", errors.New("读取 Compose 文件失败: " + err.Error())
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
	}

	content, err := os.ReadFile(cleanFile)
	if err != nil {
		logger.Logger.Error("读取 Compose 文件失败", zap.String("file", cleanFile), logger.ZapErr(err))
		return "", errors.New("读取 Compose 文件失败: " + err.Error())
	}

	return string(content), nil
}
