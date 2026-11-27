package composecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
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
	// ErrInvalidComposeFileName 表示文件名不属于标准 Compose 文件名
	ErrInvalidComposeFileName = errors.New("非法 compose 文件名")
	// ErrMultipleComposeFiles 表示传入了逗号分隔的多个 compose 文件路径
	ErrMultipleComposeFiles = errors.New("不支持同时指定多个 composeFile")
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
		return "", "", ErrMultipleComposeFiles
	}
	absFile, err := filepath.Abs(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("解析 composeFile 路径失败: %w", err)
	}
	base := filepath.Base(absFile)
	if !isComposeFileName(base) {
		return "", "", fmt.Errorf("%w: %s", ErrInvalidComposeFileName, base)
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
	primaryComposeFiles  = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
	overrideComposeFiles = []string{"compose.override.yml", "compose.override.yaml", "docker-compose.override.yml", "docker-compose.override.yaml"}
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
	for _, cf := range primaryComposeFiles {
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

// ResolveComposeTarget 解析 Compose 生命周期执行目标：
// 1. 若 composeFile 为 APP_PATH 内的合法标准文件名且文件存在，直接返回 (projectDir, nil, nil)；
// 2. 若 composeFile 位于 APP_PATH 一级子目录下但是自定义单个 .yml/.yaml 文件名，校验项目名与路径归属并查运行时记录，
//    命中后返回 (projectDir, [-p <runtimeName> -f <file1> ...], nil)；未匹配或存在歧义/错误时直接报错；
// 3. 若路径包含 NUL/.. 穿越，或位于 APP_PATH 内但为符号链接/非法目录，直接拒绝；
// 4. 若 APP_PATH 未设置或 composeFile 为空/位于 APP_PATH 之外（及列表合并后的多文件逗号路径），
//    验证 projectName 存在于 docker compose 运行时列表后返回 ("", ["-p", runtimeName], nil)。
func (c *Client) ResolveComposeTarget(ctx context.Context, projectName, composeFile string) (string, []string, error) {
	trimmedFile := strings.TrimSpace(composeFile)
	trimmedProjectName := strings.TrimSpace(projectName)
	if trimmedFile != "" {
		if strings.ContainsRune(trimmedFile, 0) {
			return "", nil, errors.New("非法的 composeFile 路径")
		}
		if strings.Contains(trimmedFile, ",") {
			for _, seg := range strings.Split(trimmedFile, ",") {
				if strings.ContainsRune(seg, 0) {
					return "", nil, errors.New("非法的 composeFile 路径")
				}
				if hasPathTraversalSegment(seg) {
					return "", nil, fmt.Errorf("%w: %s", ErrTraversalPath, trimmedFile)
				}
			}
		} else {
			projectPath, cleanFile, err := ValidateComposeFilePath(trimmedFile)
			if err == nil {
				if fi, lerr := os.Lstat(cleanFile); lerr == nil && fi.Mode().IsRegular() {
					return projectPath, nil, nil
				} else if lerr != nil && !os.IsNotExist(lerr) {
					return "", nil, fmt.Errorf("检查 composeFile 失败: %w", lerr)
				}
			} else {
				if errors.Is(err, ErrTraversalPath) {
					return "", nil, err
				}
				if isPathWithinAppRoot(trimmedFile) {
					if errors.Is(err, ErrSymlinkPath) || !errors.Is(err, ErrInvalidComposeFileName) {
						return "", nil, err
					}
					return c.resolveManagedCustomRuntimeTarget(ctx, trimmedProjectName, trimmedFile)
				}
			}
		}
	}

	if trimmedProjectName != "" {
		runtimeName, found, err := c.FindRuntimeProject(ctx, trimmedProjectName)
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

func (c *Client) resolveManagedCustomRuntimeTarget(ctx context.Context, projectName, rawComposeFile string) (string, []string, error) {
	if err := ValidateProjectName(projectName); err != nil {
		return "", nil, err
	}
	absFile, err := filepath.Abs(rawComposeFile)
	if err != nil {
		return "", nil, fmt.Errorf("解析 composeFile 路径失败: %w", err)
	}
	projectDir, err := ValidateProjectDir(filepath.Dir(absFile))
	if err != nil {
		return "", nil, err
	}
	cleanFile, err := validateProjectMemberConfigFile(projectDir, projectName, rawComposeFile)
	if err != nil {
		return "", nil, err
	}
	cand, err := c.lookupRuntimeCandidate(ctx, projectName, projectDir, cleanFile, draftSelectionExplicit)
	if err != nil {
		return "", nil, err
	}
	if cand == nil {
		return "", nil, fmt.Errorf("未找到与配置文件 %s 匹配的运行时 Compose 项目", cleanFile)
	}
	dirName := filepath.Base(projectDir)
	if !strings.EqualFold(projectName, cand.name) && !strings.EqualFold(projectName, dirName) {
		return "", nil, fmt.Errorf("项目名称 %s 与运行时项目 %s 不匹配", projectName, cand.name)
	}
	plan := DeployProjectPlan{
		ProjectDir:         projectDir,
		RuntimeProjectName: cand.name,
		DeployFiles:        cand.files,
	}
	return projectDir, plan.ComposeArgsPrefix(), nil
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

// DeployProjectPlan 描述一次保存或部署解析出的工作目录、实际写入文件、匹配的运行时项目名及有序配置文件列表。
type DeployProjectPlan struct {
	ProjectDir         string
	SavedFile          string
	RuntimeProjectName string   // 非空表示匹配到已有运行时项目（需传 -p <RuntimeProjectName>）；空表示草稿或新建项目
	DeployFiles        []string // 按顺序传给 -f 的配置文件绝对路径列表
}

// ComposeArgsPrefix 构建供 docker compose 子命令（如 up、ps）复用的 [-p <name>] -f <file1> [-f <file2> ...] 前缀参数。
func (p DeployProjectPlan) ComposeArgsPrefix() []string {
	capHint := len(p.DeployFiles) * 2
	if p.RuntimeProjectName != "" {
		capHint += 2
	}
	args := make([]string, 0, capHint)
	if p.RuntimeProjectName != "" {
		args = append(args, "-p", p.RuntimeProjectName)
	}
	for _, f := range p.DeployFiles {
		if f != "" {
			args = append(args, "-f", f)
		}
	}
	return args
}

// SaveNewProject 保存新的 Compose 项目（严格校验项目名称及位于 APP_PATH 内，拒绝符号链接）
// 当 force=false（仅保存新建草稿）时仅执行本地独占创建，不查询 Docker 运行时状态以支持离线保存。
func (c *Client) SaveNewProject(ctx context.Context, name string, yamlContent string, force bool) (string, error) {
	if force {
		return c.SaveProject(ctx, name, "", yamlContent, true)
	}
	if err := ValidateProjectName(name); err != nil {
		return "", err
	}
	appRoot, err := resolveAppRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(appRoot, 0755); err != nil {
		return "", fmt.Errorf("创建 APP_PATH %s 失败: %w", appRoot, err)
	}
	appRoot, err = resolveAppRoot()
	if err != nil {
		return "", err
	}
	projectPath, err := ValidateProjectDir(filepath.Join(appRoot, name))
	if err != nil {
		return "", err
	}

	if stat, lerr := os.Lstat(projectPath); lerr == nil {
		if stat.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: %s", ErrSymlinkPath, projectPath)
		}
		logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
		return "", errors.New("项目已存在，如需覆盖请使用强制模式")
	} else if !os.IsNotExist(lerr) {
		return "", errors.New("检查项目目录失败: " + lerr.Error())
	}

	if err := os.Mkdir(projectPath, 0755); err != nil {
		if os.IsExist(err) {
			return "", errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		logger.Logger.Error("创建项目目录失败", zap.String("path", projectPath), logger.ZapErr(err))
		return "", errors.New("创建项目目录失败: " + err.Error())
	}

	targetFile := filepath.Join(projectPath, "docker-compose.yaml")
	f, err := os.OpenFile(targetFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		_ = os.Remove(projectPath)
		if os.IsExist(err) {
			return "", errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		logger.Logger.Error("创建 Compose 文件失败", zap.String("file", targetFile), logger.ZapErr(err))
		return "", errors.New("写入 Compose 文件失败: " + err.Error())
	}

	_, writeErr := f.Write([]byte(yamlContent))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(targetFile)
		_ = os.Remove(projectPath)
		werr := writeErr
		if werr == nil {
			werr = closeErr
		}
		logger.Logger.Error("写入 Compose 文件失败", zap.String("file", targetFile), logger.ZapErr(werr))
		return "", errors.New("写入 Compose 文件失败: " + werr.Error())
	}

	logger.Logger.Info("已保存新建 Compose 草稿", zap.String("targetFile", targetFile))
	return targetFile, nil
}

// SaveProject 保存或更新指定 Compose 项目的配置文件，并返回实际写入的文件路径。
func (c *Client) SaveProject(ctx context.Context, name string, composeFile string, yamlContent string, force bool) (string, error) {
	plan, err := c.SaveProjectPlanForDeploy(ctx, name, composeFile, yamlContent, force)
	if err != nil {
		return "", err
	}
	return plan.SavedFile, nil
}

// SaveProjectForDeploy 保存编辑的 Compose 文件并返回写入路径及有序配置文件列表。
func (c *Client) SaveProjectForDeploy(ctx context.Context, name string, composeFile string, yamlContent string, force bool) (string, []string, error) {
	plan, err := c.SaveProjectPlanForDeploy(ctx, name, composeFile, yamlContent, force)
	if err != nil {
		return "", nil, err
	}
	return plan.SavedFile, plan.DeployFiles, nil
}

type runtimeCandidate struct {
	name  string
	files []string
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
		return "", fmt.Errorf("配置文件 %s 不属于项目 %s (期望目录: %s)", rawPath, projectName, projectPath)
	}
	cleanFile := filepath.Join(validDir, base)
	if fi, lerr := os.Lstat(cleanFile); lerr != nil {
		return "", fmt.Errorf("检查配置文件 %s 失败: %w", cleanFile, lerr)
	} else if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrSymlinkPath, cleanFile)
	}
	return cleanFile, nil
}

func validateExplicitComposeFileOwnership(projectPath, projectName, rawPath string) error {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return nil
	}
	if strings.ContainsRune(trimmed, 0) {
		return errors.New("非法的 composeFile 路径")
	}
	if hasPathTraversalSegment(trimmed) {
		return fmt.Errorf("%w: %s", ErrTraversalPath, rawPath)
	}
	if strings.Contains(trimmed, ",") {
		return errors.New("不支持同时指定多个 composeFile")
	}
	absFile, err := filepath.Abs(trimmed)
	if err != nil {
		return fmt.Errorf("解析 composeFile 路径失败: %w", err)
	}
	validDir, err := ensureSubdirOfAppRoot(filepath.Dir(absFile))
	if err != nil || validDir != projectPath {
		return fmt.Errorf("compose 文件 %s 不属于项目 %s", rawPath, projectName)
	}
	return nil
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

type rawRuntimeCandidate struct {
	name            string
	rawSegments     []string
	normalizedFiles []string
}

func normalizeCandidateConfigPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if resolved, err := resolvePathCheckingSymlinks(trimmed); err == nil {
		return resolved
	}
	return normalizeComposePath(trimmed)
}

func rawCandidateProjectNames(cands []rawRuntimeCandidate) []string {
	names := make([]string, len(cands))
	for i, cand := range cands {
		names[i] = cand.name
	}
	sort.Strings(names)
	return names
}

func canAdoptRuntimeFromScannedPrimaryCard(reqName, projectPath, runtimeName, cleanExplicitFile string, source draftSelectionSource) bool {
	if source != draftSelectionCard || cleanExplicitFile == "" {
		return false
	}
	if !isPrimaryComposeFileName(filepath.Base(cleanExplicitFile)) {
		return false
	}
	dirName := strings.TrimSpace(filepath.Base(projectPath))
	if dirName == "" || !strings.EqualFold(strings.TrimSpace(reqName), dirName) || !strings.EqualFold(strings.TrimSpace(runtimeName), dirName) {
		return false
	}
	primaries, err := collectExistingComposeCandidates(projectPath, primaryComposeFiles)
	return err == nil && len(primaries) == 1 && primaries[0] == cleanExplicitFile
}

func (c *Client) lookupRuntimeCandidate(ctx context.Context, name, projectPath, cleanExplicitFile string, source draftSelectionSource) (*runtimeCandidate, error) {
	runtimeList, err := c.listRuntimeProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询运行时 Compose 项目失败: %w", err)
	}

	// 第一步：仅按记录中的项目名与规范化路径匹配和消除歧义，不提前 Lstat 未选中兄弟项目的文件
	var rawDirCandidates []rawRuntimeCandidate
	for _, item := range runtimeList {
		rawConfigs := strings.TrimSpace(item.ConfigFiles)
		if rawConfigs == "" {
			continue
		}
		var rawSegments []string
		var normalizedFiles []string
		belongsToDir := false
		for _, seg := range strings.Split(rawConfigs, ",") {
			trimmedSeg := strings.TrimSpace(seg)
			if trimmedSeg == "" {
				continue
			}
			rawSegments = append(rawSegments, trimmedSeg)
			norm := normalizeCandidateConfigPath(trimmedSeg)
			normalizedFiles = append(normalizedFiles, norm)
			if norm != "" && filepath.Dir(norm) == projectPath {
				belongsToDir = true
			}
		}
		if !belongsToDir {
			continue
		}
		rawDirCandidates = append(rawDirCandidates, rawRuntimeCandidate{
			name:            strings.TrimSpace(item.Name),
			rawSegments:     rawSegments,
			normalizedFiles: normalizedFiles,
		})
	}

	if len(rawDirCandidates) == 0 {
		trimmedName := strings.TrimSpace(name)
		for _, item := range runtimeList {
			if !strings.EqualFold(strings.TrimSpace(item.Name), trimmedName) {
				continue
			}
			rawConfigs := strings.TrimSpace(item.ConfigFiles)
			if rawConfigs == "" {
				continue
			}
			for _, seg := range strings.Split(rawConfigs, ",") {
				if strings.TrimSpace(seg) == "" {
					continue
				}
				if _, err := validateProjectMemberConfigFile(projectPath, name, seg); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	}

	var chosen rawRuntimeCandidate
	matchedViaScannedPrimary := false
	if len(rawDirCandidates) == 1 {
		cand := rawDirCandidates[0]
		if cleanExplicitFile != "" && !slices.Contains(cand.normalizedFiles, cleanExplicitFile) {
			if canAdoptRuntimeFromScannedPrimaryCard(name, projectPath, cand.name, cleanExplicitFile, source) {
				matchedViaScannedPrimary = true
			} else {
				return nil, fmt.Errorf("目标配置文件 %s 与运行时项目 %s 的配置文件列表 [%s] 冲突", cleanExplicitFile, cand.name, strings.Join(cand.normalizedFiles, ", "))
			}
		}
		chosen = cand
	} else {
		allNames := rawCandidateProjectNames(rawDirCandidates)
		if cleanExplicitFile == "" {
			return nil, fmt.Errorf("项目目录 %s 存在多个运行时 Compose 项目 (%s)，请指定对应配置文件以消除歧义", projectPath, strings.Join(allNames, ", "))
		}

		var fileMatches []rawRuntimeCandidate
		for _, cand := range rawDirCandidates {
			if slices.Contains(cand.normalizedFiles, cleanExplicitFile) {
				fileMatches = append(fileMatches, cand)
			}
		}
		if len(fileMatches) == 0 {
			return nil, fmt.Errorf("目标配置文件 %s 与目录中多个运行时项目 (%s) 均不匹配", cleanExplicitFile, strings.Join(allNames, ", "))
		}
		if len(fileMatches) == 1 {
			chosen = fileMatches[0]
		} else {
			trimmedName := strings.TrimSpace(name)
			var nameMatches []rawRuntimeCandidate
			for _, cand := range fileMatches {
				if strings.EqualFold(cand.name, trimmedName) {
					nameMatches = append(nameMatches, cand)
				}
			}
			if len(nameMatches) != 1 {
				return nil, fmt.Errorf("项目目录 %s 中配置文件 %s 匹配多个运行时 Compose 项目 (%s)，无法唯一定位",
					projectPath, cleanExplicitFile, strings.Join(rawCandidateProjectNames(fileMatches), ", "))
			}
			chosen = nameMatches[0]
		}
	}

	// 第二步：唯一定位目标后，仅对选中项目的全部配置文件执行存在性、归属和符号链接校验
	if err := ValidateProjectName(chosen.name); err != nil {
		return nil, fmt.Errorf("运行时项目名 %q 非法: %w", chosen.name, err)
	}
	files := make([]string, 0, len(chosen.rawSegments))
	seen := make(map[string]struct{}, len(chosen.rawSegments))
	for _, seg := range chosen.rawSegments {
		cleanFile, err := validateProjectMemberConfigFile(projectPath, name, seg)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cleanFile]; ok {
			continue
		}
		seen[cleanFile] = struct{}{}
		files = append(files, cleanFile)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("运行时项目 %s 缺少有效的配置文件", chosen.name)
	}
	if cleanExplicitFile != "" && !matchedViaScannedPrimary && !slices.Contains(files, cleanExplicitFile) {
		return nil, fmt.Errorf("目标配置文件 %s 与运行时项目 %s 的配置文件列表 [%s] 冲突", cleanExplicitFile, chosen.name, strings.Join(files, ", "))
	}
	return &runtimeCandidate{
		name:  chosen.name,
		files: files,
	}, nil
}

func parseDotEnvFileInto(envPath string, requireExists bool, envMap map[string]string) error {
	fi, err := os.Lstat(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			if requireExists {
				return fmt.Errorf("环境文件不存在: %s", envPath)
			}
			return nil
		}
		return fmt.Errorf("检查环境文件失败: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrSymlinkPath, envPath)
	}

	raw, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("读取环境文件失败: %w", err)
	}

	next := maps.Clone(envMap)
	if next == nil {
		next = make(map[string]string)
	}
	lookup := func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
		value, ok := next[key]
		return value, ok
	}
	if err := dotenv.ParseWithFormat(bytes.NewReader(raw), envPath, next, lookup, dotenv.DotEnv); err != nil {
		return fmt.Errorf("解析环境文件 %s 失败: %w", envPath, err)
	}
	maps.Copy(envMap, next)
	return nil
}

func resolveDraftEnvComposeFiles(projectPath, projectName string) ([]string, bool, error) {
	dotEnv := make(map[string]string)
	if rawEnvFiles, hasEnvFilesEnv := os.LookupEnv("COMPOSE_ENV_FILES"); hasEnvFilesEnv && strings.TrimSpace(rawEnvFiles) != "" {
		for _, part := range strings.Split(rawEnvFiles, ",") {
			trimmedEnv := strings.TrimSpace(part)
			if trimmedEnv == "" {
				return nil, false, fmt.Errorf("COMPOSE_ENV_FILES 配置 %q 包含空路径项", rawEnvFiles)
			}
			if strings.ContainsRune(trimmedEnv, 0) {
				return nil, false, fmt.Errorf("非法的环境文件路径: %s", trimmedEnv)
			}
			envPath := trimmedEnv
			if !filepath.IsAbs(envPath) {
				envPath = filepath.Join(projectPath, envPath)
			}
			if err := parseDotEnvFileInto(envPath, true, dotEnv); err != nil {
				return nil, false, err
			}
		}
	} else {
		disabled := false
		if rawDisable, hasDisable := os.LookupEnv("COMPOSE_DISABLE_ENV_FILE"); hasDisable {
			if b, berr := strconv.ParseBool(strings.TrimSpace(rawDisable)); berr == nil {
				disabled = b
			}
		}
		if !disabled {
			if err := parseDotEnvFileInto(filepath.Join(projectPath, ".env"), false, dotEnv); err != nil {
				return nil, false, err
			}
		}
	}

	sep, hasSepEnv := os.LookupEnv("COMPOSE_PATH_SEPARATOR")
	if !hasSepEnv {
		sep = dotEnv["COMPOSE_PATH_SEPARATOR"]
	}
	if sep == "" {
		sep = string(os.PathListSeparator)
	}

	rawComposeFile, present := os.LookupEnv("COMPOSE_FILE")
	if !present {
		rawComposeFile, present = dotEnv["COMPOSE_FILE"]
	}
	if !present {
		return nil, false, nil
	}
	rawComposeFile = strings.TrimSpace(rawComposeFile)
	if rawComposeFile == "" {
		return nil, false, errors.New("COMPOSE_FILE 已设置，但未指定配置文件")
	}

	parts := strings.Split(rawComposeFile, sep)
	files := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart == "" {
			return nil, false, fmt.Errorf("COMPOSE_FILE 配置 %q 包含空路径项", rawComposeFile)
		}
		if hasPathTraversalSegment(trimmedPart) {
			return nil, false, fmt.Errorf("%w: %s", ErrTraversalPath, trimmedPart)
		}
		candidate := trimmedPart
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(projectPath, candidate)
		}
		cleanFile, err := validateProjectMemberConfigFile(projectPath, projectName, candidate)
		if err != nil {
			return nil, false, err
		}
		if _, ok := seen[cleanFile]; ok {
			continue
		}
		seen[cleanFile] = struct{}{}
		files = append(files, cleanFile)
	}
	return files, len(files) > 0, nil
}

func (c *Client) resolveDeployPlan(ctx context.Context, name, composeFile string, forWrite bool, force bool) (DeployProjectPlan, bool, error) {
	if err := ValidateProjectName(name); err != nil {
		return DeployProjectPlan{}, false, err
	}
	appRoot, err := resolveAppRoot()
	if err != nil {
		return DeployProjectPlan{}, false, err
	}

	projectPath, err := ValidateProjectDir(filepath.Join(appRoot, name))
	if err != nil {
		return DeployProjectPlan{}, false, err
	}

	dirExists := false
	if stat, err := os.Lstat(projectPath); err == nil {
		if stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
			return DeployProjectPlan{}, false, fmt.Errorf("%w: %s", ErrSymlinkPath, projectPath)
		}
		dirExists = true
	} else if !os.IsNotExist(err) {
		return DeployProjectPlan{}, false, errors.New("检查项目目录失败: " + err.Error())
	}

	trimmedFile := strings.TrimSpace(composeFile)
	var cleanExplicitFile string
	if trimmedFile != "" {
		if err := validateExplicitComposeFileOwnership(projectPath, name, trimmedFile); err != nil {
			return DeployProjectPlan{}, false, err
		}
		if !dirExists {
			return DeployProjectPlan{}, false, errors.New("项目目录不存在")
		}
		cleanExplicitFile, err = validateProjectMemberConfigFile(projectPath, name, trimmedFile)
		if err != nil {
			return DeployProjectPlan{}, false, err
		}
	} else if !forWrite && !dirExists {
		return DeployProjectPlan{}, false, errors.New("项目目录不存在")
	}

	// Tier 1: 查询运行时项目列表（查询失败直接报错，绝不静默降级为草稿）
	runtimeCand, err := c.lookupRuntimeCandidate(ctx, name, projectPath, cleanExplicitFile, draftSelectionExplicit)
	if err != nil {
		return DeployProjectPlan{}, false, err
	}
	if runtimeCand != nil {
		if forWrite && !force {
			logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
			return DeployProjectPlan{}, false, errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		savedFile := cleanExplicitFile
		if forWrite && savedFile == "" {
			if len(runtimeCand.files) > 1 {
				return DeployProjectPlan{}, false, fmt.Errorf("运行时项目 %s 包含多个配置文件 [%s]，请指定 composeFile", runtimeCand.name, strings.Join(runtimeCand.files, ", "))
			}
			savedFile = runtimeCand.files[0]
		}
		return DeployProjectPlan{
			ProjectDir:         projectPath,
			SavedFile:          savedFile,
			RuntimeProjectName: runtimeCand.name,
			DeployFiles:        runtimeCand.files,
		}, dirExists, nil
	}

	if !dirExists {
		targetFile := filepath.Join(projectPath, "docker-compose.yaml")
		return DeployProjectPlan{
			ProjectDir:  projectPath,
			SavedFile:   targetFile,
			DeployFiles: []string{targetFile},
		}, false, nil
	}

	if forWrite && !force {
		logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
		return DeployProjectPlan{}, false, errors.New("项目已存在，如需覆盖请使用强制模式")
	}

	// Tier 2 + Tier 3: 复用无运行时记录时的草稿部署文件解析逻辑
	plan, err := resolveDraftDeployPlan(projectPath, name, cleanExplicitFile, draftSelectionExplicit, forWrite)
	if err != nil {
		return DeployProjectPlan{}, false, err
	}
	return plan, true, nil
}

type draftSelectionSource uint8

const (
	draftSelectionExplicit draftSelectionSource = iota
	draftSelectionCard
)

func resolveDraftDeployPlan(projectPath, name, cleanExplicitFile string, source draftSelectionSource, forWrite bool) (DeployProjectPlan, error) {
	// Tier 2: 草稿目录若在 .env（或进程环境）中声明了 COMPOSE_FILE，遵循其文件列表与顺序
	envFiles, hasEnvFiles, err := resolveDraftEnvComposeFiles(projectPath, name)
	if err != nil {
		return DeployProjectPlan{}, err
	}
	if hasEnvFiles {
		adoptEnvFilesFromCard := !forWrite && source == draftSelectionCard && cleanExplicitFile != "" && isPrimaryComposeFileName(filepath.Base(cleanExplicitFile))
		if cleanExplicitFile != "" && !adoptEnvFilesFromCard && !slices.Contains(envFiles, cleanExplicitFile) {
			return DeployProjectPlan{}, fmt.Errorf("目标配置文件 %s 与 COMPOSE_FILE 配置文件列表 [%s] 冲突", cleanExplicitFile, strings.Join(envFiles, ", "))
		}
		savedFile := cleanExplicitFile
		if adoptEnvFilesFromCard {
			savedFile = ""
		}
		if forWrite && savedFile == "" {
			if len(envFiles) > 1 {
				return DeployProjectPlan{}, fmt.Errorf("COMPOSE_FILE 包含多个配置文件 [%s]，请指定 composeFile", strings.Join(envFiles, ", "))
			}
			savedFile = envFiles[0]
		}
		return DeployProjectPlan{
			ProjectDir:  projectPath,
			SavedFile:   savedFile,
			DeployFiles: envFiles,
		}, nil
	}

	// Tier 3: 无运行时记录且无 COMPOSE_FILE 时按目录发现草稿文件；显式选择主文件时直接采用用户选择
	if cleanExplicitFile != "" && !isComposeFileName(filepath.Base(cleanExplicitFile)) {
		return DeployProjectPlan{}, fmt.Errorf("%w: %s", ErrInvalidComposeFileName, filepath.Base(cleanExplicitFile))
	}
	primaries, err := collectExistingComposeCandidates(projectPath, primaryComposeFiles)
	if err != nil {
		return DeployProjectPlan{}, err
	}
	overrides, err := collectExistingComposeCandidates(projectPath, overrideComposeFiles)
	if err != nil {
		return DeployProjectPlan{}, err
	}

	if cleanExplicitFile != "" {
		baseName := filepath.Base(cleanExplicitFile)
		if isPrimaryComposeFileName(baseName) {
			deployFiles := []string{cleanExplicitFile}
			if len(overrides) > 0 {
				deployFiles = append(deployFiles, overrides[0])
			}
			return DeployProjectPlan{
				ProjectDir:  projectPath,
				SavedFile:   cleanExplicitFile,
				DeployFiles: deployFiles,
			}, nil
		}
		if len(primaries) == 0 {
			return DeployProjectPlan{}, fmt.Errorf("项目目录 %s 中未找到主 Compose 配置文件", projectPath)
		}
		if len(primaries) > 1 {
			return DeployProjectPlan{}, fmt.Errorf("项目目录存在多个 Compose 主配置文件 (%s)，请明确选择主配置文件", strings.Join(primaries, ", "))
		}
		if len(overrides) > 0 && overrides[0] != cleanExplicitFile {
			return DeployProjectPlan{}, fmt.Errorf("编辑目标覆盖文件 %s 与默认生效覆盖文件 %s 冲突", cleanExplicitFile, overrides[0])
		}
		return DeployProjectPlan{
			ProjectDir:  projectPath,
			SavedFile:   cleanExplicitFile,
			DeployFiles: []string{primaries[0], cleanExplicitFile},
		}, nil
	}

	if len(primaries) > 1 {
		return DeployProjectPlan{}, fmt.Errorf("项目目录存在多个 Compose 主配置文件 (%s)，请指定 composeFile", strings.Join(primaries, ", "))
	}
	if len(primaries) == 1 {
		deployFiles := []string{primaries[0]}
		if len(overrides) > 0 {
			deployFiles = append(deployFiles, overrides[0])
		}
		return DeployProjectPlan{
			ProjectDir:  projectPath,
			SavedFile:   primaries[0],
			DeployFiles: deployFiles,
		}, nil
	}

	if !forWrite {
		return DeployProjectPlan{}, fmt.Errorf("项目目录 %s 中未找到 Compose 配置文件", projectPath)
	}
	targetFile := filepath.Join(projectPath, "docker-compose.yaml")
	deployFiles := []string{targetFile}
	if len(overrides) > 0 {
		deployFiles = append(deployFiles, overrides[0])
	}
	return DeployProjectPlan{
		ProjectDir:  projectPath,
		SavedFile:   targetFile,
		DeployFiles: deployFiles,
	}, nil
}

// ResolveManagedCardPlan 解析卡片“创建”与“拉取镜像”操作的目标目录、运行时项目名及有序配置文件列表。
// 要求所有传入文件均经过校验并严格属于同一个 APP_PATH 一级受管子目录；
// 若命中运行时记录则验证完整顺序并返回其真实项目名与全部文件；若为无运行时记录的单文件草稿则复用既有草稿解析逻辑。
func (c *Client) ResolveManagedCardPlan(ctx context.Context, projectName, composeFiles string) (DeployProjectPlan, error) {
	trimmedName := strings.TrimSpace(projectName)
	if err := ValidateProjectName(trimmedName); err != nil {
		return DeployProjectPlan{}, err
	}
	trimmedFiles := strings.TrimSpace(composeFiles)
	if trimmedFiles == "" {
		return DeployProjectPlan{}, errors.New("composeFile 不能为空")
	}
	if strings.ContainsRune(trimmedFiles, 0) {
		return DeployProjectPlan{}, errors.New("非法的 composeFile 路径")
	}

	rawParts := strings.Split(trimmedFiles, ",")
	selectedFiles := make([]string, 0, len(rawParts))
	var projectDir string
	for _, part := range rawParts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart == "" {
			return DeployProjectPlan{}, errors.New("composeFile 列表包含空路径项")
		}
		if hasPathTraversalSegment(trimmedPart) {
			return DeployProjectPlan{}, fmt.Errorf("%w: %s", ErrTraversalPath, trimmedPart)
		}
		absPart, err := filepath.Abs(trimmedPart)
		if err != nil {
			return DeployProjectPlan{}, fmt.Errorf("解析 composeFile 路径失败: %w", err)
		}
		partDir, err := ValidateProjectDir(filepath.Dir(absPart))
		if err != nil {
			return DeployProjectPlan{}, err
		}
		if projectDir == "" {
			projectDir = partDir
		} else if partDir != projectDir {
			return DeployProjectPlan{}, fmt.Errorf("composeFile 列表中的文件 %s 不属于同一受管目录 %s", trimmedPart, projectDir)
		}
		cleanFile, err := validateProjectMemberConfigFile(projectDir, trimmedName, trimmedPart)
		if err != nil {
			return DeployProjectPlan{}, err
		}
		if slices.Contains(selectedFiles, cleanFile) {
			return DeployProjectPlan{}, fmt.Errorf("composeFile 列表包含重复文件: %s", cleanFile)
		}
		selectedFiles = append(selectedFiles, cleanFile)
	}
	if projectDir == "" || len(selectedFiles) == 0 {
		return DeployProjectPlan{}, errors.New("composeFile 不能为空")
	}

	dirName := filepath.Base(projectDir)
	cardSource := draftSelectionExplicit
	if len(selectedFiles) == 1 {
		cardSource = draftSelectionCard
	}
	cand, err := c.lookupRuntimeCandidate(ctx, trimmedName, projectDir, selectedFiles[0], cardSource)
	if err != nil {
		return DeployProjectPlan{}, err
	}
	if cand != nil {
		if !strings.EqualFold(trimmedName, cand.name) && !strings.EqualFold(trimmedName, dirName) {
			return DeployProjectPlan{}, fmt.Errorf("项目名称 %s 与运行时项目 %s 不匹配", trimmedName, cand.name)
		}
		if len(selectedFiles) > 1 && !slices.Equal(selectedFiles, cand.files) {
			return DeployProjectPlan{}, fmt.Errorf("指定的配置文件列表 [%s] 与运行时项目 %s 的配置文件 [%s] 不一致",
				strings.Join(selectedFiles, ", "), cand.name, strings.Join(cand.files, ", "))
		}
		return DeployProjectPlan{
			ProjectDir:         projectDir,
			RuntimeProjectName: cand.name,
			DeployFiles:        cand.files,
		}, nil
	}

	if len(selectedFiles) > 1 {
		return DeployProjectPlan{}, fmt.Errorf("未找到与多文件配置 [%s] 匹配的运行时 Compose 项目", strings.Join(selectedFiles, ", "))
	}
	if !strings.EqualFold(trimmedName, dirName) {
		return DeployProjectPlan{}, fmt.Errorf("草稿项目名称 %s 与目录名称 %s 不匹配", trimmedName, dirName)
	}

	plan, err := resolveDraftDeployPlan(projectDir, dirName, selectedFiles[0], draftSelectionCard, false)
	if err != nil {
		return DeployProjectPlan{}, err
	}
	if plan.ProjectDir == "" || len(plan.DeployFiles) == 0 {
		return DeployProjectPlan{}, fmt.Errorf("项目目录 %s 中未找到有效的 Compose 配置文件", projectDir)
	}
	return plan, nil
}

// ResolveDeployPlan 解析无需重写 YAML 时的完整部署方案（含运行时项目名和有序 -f 文件列表）。
func (c *Client) ResolveDeployPlan(ctx context.Context, name, composeFile string) (DeployProjectPlan, error) {
	plan, _, err := c.resolveDeployPlan(ctx, name, composeFile, false, false)
	return plan, err
}

// ResolveDeployProjectFiles 解析无需重写 YAML 时的有序部署配置文件列表并校验目标归属。
func (c *Client) ResolveDeployProjectFiles(ctx context.Context, name, composeFile string) (string, []string, error) {
	plan, err := c.ResolveDeployPlan(ctx, name, composeFile)
	if err != nil {
		return "", nil, err
	}
	return plan.ProjectDir, plan.DeployFiles, nil
}

// SaveProjectPlanForDeploy 保存用户编辑的单个 Compose 文件并返回完整部署方案（含匹配的运行时项目名与有序配置文件列表）。
func (c *Client) SaveProjectPlanForDeploy(ctx context.Context, name string, composeFile string, yamlContent string, force bool) (DeployProjectPlan, error) {
	plan, dirExists, err := c.resolveDeployPlan(ctx, name, composeFile, true, force)
	if err != nil {
		return DeployProjectPlan{}, err
	}

	if !dirExists {
		if err := os.MkdirAll(plan.ProjectDir, 0755); err != nil {
			logger.Logger.Error("创建项目目录失败", zap.String("path", plan.ProjectDir), logger.ZapErr(err))
			return DeployProjectPlan{}, errors.New("创建项目目录失败: " + err.Error())
		}
	}

	if fi, err := os.Lstat(plan.SavedFile); err == nil && (fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular()) {
		return DeployProjectPlan{}, fmt.Errorf("%w: %s", ErrSymlinkPath, plan.SavedFile)
	} else if err != nil && !os.IsNotExist(err) {
		return DeployProjectPlan{}, errors.New("检查 Compose 文件失败: " + err.Error())
	}

	if err := os.WriteFile(plan.SavedFile, []byte(yamlContent), 0644); err != nil {
		logger.Logger.Error("写入 Compose 文件失败", zap.String("file", plan.SavedFile), logger.ZapErr(err))
		return DeployProjectPlan{}, errors.New("写入 Compose 文件失败: " + err.Error())
	}

	logger.Logger.Info("已保存 Compose 配置文件",
		zap.String("targetFile", plan.SavedFile),
		zap.String("runtimeProjectName", plan.RuntimeProjectName),
		zap.Strings("deployFiles", plan.DeployFiles))
	return plan, nil
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
