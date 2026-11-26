package composecli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

func TestMergeComposeProjectsMatchesConfigPathAndPreservesDirectoryName(t *testing.T) {
	scanned := []ComposeProject{{
		Name:        "Watch-Docker",
		ComposeFile: "/apps/Watch-Docker/docker-compose.yml",
		Status:      StatusDraft,
	}}
	runtime := []ListOutput{{
		Name:        "watch-docker",
		Status:      "running(2),exited(1)",
		ConfigFiles: "/apps/Watch-Docker/docker-compose.yml",
	}}

	got := mergeComposeProjects(scanned, runtime)
	if len(got) != 1 {
		t.Fatalf("expected one merged project, got %d: %#v", len(got), got)
	}
	if got[0].Name != "Watch-Docker" {
		t.Fatalf("expected original directory name, got %q", got[0].Name)
	}
	if got[0].ComposeFile != scanned[0].ComposeFile {
		t.Fatalf("expected scanned compose path %q, got %q", scanned[0].ComposeFile, got[0].ComposeFile)
	}
	if got[0].Status != StatusPartial || got[0].RunningCount != 2 || got[0].ExitedCount != 1 {
		t.Fatalf("expected runtime status and counts to be merged, got %#v", got[0])
	}
}

func TestMergeComposeProjectsMatchesAnyRuntimeConfigPath(t *testing.T) {
	scanned := []ComposeProject{{
		Name:        "Watch-Docker",
		ComposeFile: "/apps/Watch-Docker/compose.yaml",
		Status:      StatusDraft,
	}}
	runtime := []ListOutput{{
		Name:        "watch-docker",
		Status:      "running",
		ConfigFiles: "/tmp/override.yaml,/apps/Watch-Docker/compose.yaml",
	}}

	got := mergeComposeProjects(scanned, runtime)
	if len(got) != 1 || got[0].Name != "Watch-Docker" || got[0].Status != StatusRunning {
		t.Fatalf("expected comma-separated config path match, got %#v", got)
	}
}

func TestMergeComposeProjectsUsesUniqueCaseInsensitiveFallback(t *testing.T) {
	scanned := []ComposeProject{{
		Name:        "Watch-Docker",
		ComposeFile: "/apps/Watch-Docker/compose.yaml",
		Status:      StatusDraft,
	}}
	runtime := []ListOutput{{
		Name:        "watch-docker",
		Status:      "exited",
		ConfigFiles: "/unavailable/compose.yaml",
	}}

	got := mergeComposeProjects(scanned, runtime)
	if len(got) != 1 || got[0].Name != "Watch-Docker" || got[0].Status != StatusExited {
		t.Fatalf("expected unique name fallback to preserve scanned project, got %#v", got)
	}
}

func TestMergeComposeProjectsDoesNotCollapseAmbiguousNames(t *testing.T) {
	scanned := []ComposeProject{
		{Name: "Watch-Docker", ComposeFile: "/apps/one/compose.yaml", Status: StatusDraft},
		{Name: "watch-docker", ComposeFile: "/apps/two/compose.yaml", Status: StatusDraft},
	}
	runtime := []ListOutput{{
		Name:        "WATCH-DOCKER",
		Status:      "running",
		ConfigFiles: "/unavailable/compose.yaml",
	}}

	got := mergeComposeProjects(scanned, runtime)
	if len(got) != 3 {
		t.Fatalf("expected ambiguous runtime project to remain separate, got %d: %#v", len(got), got)
	}
}

func TestMergeComposeProjectsAppendsUnmatchedRuntimeProject(t *testing.T) {
	runtime := []ListOutput{{
		Name:        "runtime-only",
		Status:      "created(3)",
		ConfigFiles: "/apps/runtime-only/compose.yaml",
	}}

	got := mergeComposeProjects(nil, runtime)
	if len(got) != 1 {
		t.Fatalf("expected runtime project, got %#v", got)
	}
	if got[0].Name != "runtime-only" || got[0].ComposeFile != runtime[0].ConfigFiles {
		t.Fatalf("unexpected runtime project identity: %#v", got[0])
	}
	if got[0].Status != StatusCreatedStack || got[0].CreatedCount != 3 {
		t.Fatalf("unexpected runtime project status: %#v", got[0])
	}
}

func TestComposePathTraversalBlocked(t *testing.T) {
	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	outsideDir := filepath.Join(tmpRoot, "outside-secret")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsideCompose := filepath.Join(outsideDir, "docker-compose.yaml")
	if err := os.WriteFile(outsideCompose, []byte("services: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origAppPath := conf.EnvCfg.APP_PATH
	conf.EnvCfg.APP_PATH = appDir
	defer func() { conf.EnvCfg.APP_PATH = origAppPath }()

	c := &Client{}

	// 1. ValidateProjectName rejects traversal
	for _, bad := range []string{"", "..", "../evil", "a/b", "a\\b", "-flag"} {
		if err := ValidateProjectName(bad); err == nil {
			t.Fatalf("expected ValidateProjectName(%q) to fail", bad)
		}
	}
	if _, err := c.SaveNewProject(context.Background(), "../../outside-secret", "services: {}\n", true); err == nil {
		t.Fatalf("expected SaveNewProject traversal to fail")
	}

	// 2. GetProjectYaml / DeleteProject reject outside path
	if _, err := c.GetProjectYaml(outsideCompose); err == nil {
		t.Fatalf("expected GetProjectYaml outside APP_PATH to fail")
	}
	if err := c.DeleteProject(context.Background(), "", outsideCompose, StatusDraft); err == nil {
		t.Fatalf("expected DeleteProject outside APP_PATH to fail")
	}
	if _, err := os.Stat(outsideCompose); err != nil {
		t.Fatalf("expected outside compose directory to remain intact, stat err=%v", err)
	}

	// 3. Root APP_PATH and nested depth-2 subdirs cannot be used/deleted
	rootCompose := filepath.Join(appDir, "docker-compose.yaml")
	if _, _, err := ValidateComposeFilePath(rootCompose); err == nil {
		t.Fatalf("expected compose file directly under APP_PATH root to be rejected")
	}
	nestedCompose := filepath.Join(appDir, "a", "b", "docker-compose.yaml")
	if _, _, err := ValidateComposeFilePath(nestedCompose); err == nil {
		t.Fatalf("expected nested depth-2 compose path under APP_PATH to be rejected")
	}

	// 4. Valid project subdir succeeds
	saved, err := c.SaveNewProject(context.Background(), "valid-app", "services: {}\n", false)
	if err != nil {
		t.Fatalf("expected valid SaveNewProject to succeed, got %v", err)
	}
	if _, _, err := ValidateComposeFilePath(saved); err != nil {
		t.Fatalf("expected ValidateComposeFilePath(%q) to succeed, got %v", saved, err)
	}

	// 5. Symlinked compose file and symlinked project dir are rejected
	symDir := filepath.Join(appDir, "sym-proj")
	if err := os.MkdirAll(symDir, 0755); err != nil {
		t.Fatal(err)
	}
	symCompose := filepath.Join(symDir, "docker-compose.yaml")
	if err := os.Symlink(outsideCompose, symCompose); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateComposeFilePath(symCompose); err == nil {
		t.Fatalf("expected symlink compose file to be rejected")
	}
	if _, err := c.GetProjectYaml(symCompose); err == nil {
		t.Fatalf("expected GetProjectYaml on symlink compose file to fail")
	}
	if err := c.DeleteProject(context.Background(), "sym-proj", symCompose, StatusDraft); err == nil {
		t.Fatalf("expected DeleteProject(StatusDraft) on symlink compose file to fail")
	}
	symProjDir := filepath.Join(appDir, "sym-dir-link")
	if err := os.Symlink(outsideDir, symProjDir); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateProjectDir(symProjDir); err == nil {
		t.Fatalf("expected symlink project directory to be rejected")
	}

	// 6. Custom filename inside APP_PATH fails immediately while outside path checks runtime
	insideCustom := filepath.Join(appDir, "valid-app", "docker-compose.prod.yml")
	if !isPathWithinAppRoot(insideCustom) {
		t.Fatalf("expected inside custom compose filename to be treated as inside APP_PATH")
	}
	outsideCustom := filepath.Join(outsideDir, "docker-compose.prod.yml")
	if isPathWithinAppRoot(outsideCustom) {
		t.Fatalf("expected outside custom compose filename not to be treated as inside APP_PATH")
	}
}

func TestSaveProjectPreservesSiblingComposeFileAndValidatesOwnership(t *testing.T) {
	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	projADir := filepath.Join(appDir, "proj-a")
	projBDir := filepath.Join(appDir, "proj-b")
	for _, dir := range []string{projADir, projBDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = appDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
	})

	composeYamlPath := filepath.Join(projADir, "compose.yaml")
	dockerComposeYmlPath := filepath.Join(projADir, "docker-compose.yml")
	origComposeYaml := "services:\n  web:\n    image: nginx:1.25\n"
	origDockerComposeYml := "services:\n  legacy:\n    image: busybox:1.36\n"
	if err := os.WriteFile(composeYamlPath, []byte(origComposeYaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerComposeYmlPath, []byte(origDockerComposeYml), 0644); err != nil {
		t.Fatal(err)
	}

	projBFile := filepath.Join(projBDir, "compose.yaml")
	origProjB := "services:\n  b:\n    image: alpine:3.20\n"
	if err := os.WriteFile(projBFile, []byte(origProjB), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Client{}
	ctx := context.Background()

	// 1. 显式指定 compose.yaml 时只覆盖 compose.yaml，不改动 docker-compose.yml
	updatedYaml := "services:\n  web:\n    image: nginx:1.27\n"
	savedPath, err := c.SaveProject(ctx, "proj-a", composeYamlPath, updatedYaml, true)
	if err != nil {
		t.Fatalf("expected SaveProject on compose.yaml to succeed, got %v", err)
	}
	gotComposeYaml, err := os.ReadFile(composeYamlPath)
	if err != nil || string(gotComposeYaml) != updatedYaml {
		t.Fatalf("compose.yaml content mismatch: got=%q err=%v", string(gotComposeYaml), err)
	}
	if gotSavedPath, err := filepath.EvalSymlinks(savedPath); err != nil {
		t.Fatalf("eval saved path: %v", err)
	} else if wantPath, _ := filepath.EvalSymlinks(composeYamlPath); gotSavedPath != wantPath {
		t.Fatalf("saved path mismatch: got %q, want %q", savedPath, wantPath)
	}
	gotLegacyYml, err := os.ReadFile(dockerComposeYmlPath)
	if err != nil || string(gotLegacyYml) != origDockerComposeYml {
		t.Fatalf("docker-compose.yml must stay untouched: got=%q want=%q err=%v", string(gotLegacyYml), origDockerComposeYml, err)
	}

	// 2. 旧客户端省略 composeFile 且目录有多个合法配置文件时，写入前拒绝
	if _, err := c.SaveProject(ctx, "proj-a", "", "services:\n  bad: {}\n", true); err == nil {
		t.Fatalf("expected error when composeFile omitted on multi-file directory")
	}
	if gotAfter, _ := os.ReadFile(composeYamlPath); string(gotAfter) != updatedYaml {
		t.Fatalf("compose.yaml modified after multi-file error: %q", string(gotAfter))
	}
	if gotLegacyAfter, _ := os.ReadFile(dockerComposeYmlPath); string(gotLegacyAfter) != origDockerComposeYml {
		t.Fatalf("docker-compose.yml modified after multi-file error: %q", string(gotLegacyAfter))
	}

	// 3. 指定属于其他项目的 composeFile 时拒绝且不修改任何文件
	if _, err := c.SaveProject(ctx, "proj-a", projBFile, "services:\n  hijack: {}\n", true); err == nil {
		t.Fatalf("expected cross-project composeFile to be rejected")
	}
	if gotProjB, _ := os.ReadFile(projBFile); string(gotProjB) != origProjB {
		t.Fatalf("proj-b/compose.yaml modified after cross-project rejection: %q", string(gotProjB))
	}

	// 4. 旧客户端省略 composeFile 且目录仅有一份合法配置时可直接写入
	singleDir := filepath.Join(appDir, "single-proj")
	if err := os.MkdirAll(singleDir, 0755); err != nil {
		t.Fatal(err)
	}
	singleFile := filepath.Join(singleDir, "compose.yaml")
	if err := os.WriteFile(singleFile, []byte(origComposeYaml), 0644); err != nil {
		t.Fatal(err)
	}
	singleSaved, err := c.SaveProject(ctx, "single-proj", "", updatedYaml, true)
	if err != nil {
		t.Fatalf("expected single-file legacy save to succeed, got %v", err)
	}
	if wantSingle, _ := filepath.EvalSymlinks(singleFile); singleSaved != wantSingle {
		t.Fatalf("expected singleSaved %q, got %q", wantSingle, singleSaved)
	}
	if gotSingle, _ := os.ReadFile(singleFile); string(gotSingle) != updatedYaml {
		t.Fatalf("single compose.yaml not updated: %q", string(gotSingle))
	}

	// 5. 编辑目录发现项目中的标准 override 文件时，保留主文件 + override 的完整有序部署列表
	overrideDir := filepath.Join(appDir, "with-override")
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		t.Fatal(err)
	}
	baseYaml := filepath.Join(overrideDir, "compose.yaml")
	overrideYaml := filepath.Join(overrideDir, "compose.override.yaml")
	origBase := "services:\n  api:\n    image: nginx:1.25\n"
	origOverride := "services:\n  worker:\n    image: alpine:3.19\n"
	updatedOverride := "services:\n  worker:\n    image: alpine:3.20\n"
	if err := os.WriteFile(baseYaml, []byte(origBase), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overrideYaml, []byte(origOverride), 0644); err != nil {
		t.Fatal(err)
	}
	overrideCli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return nil, nil
		},
	}
	savedOv, deployFilesOv, err := overrideCli.SaveProjectForDeploy(ctx, "with-override", overrideYaml, updatedOverride, true)
	if err != nil {
		t.Fatalf("expected editing override file to succeed, got %v", err)
	}
	resolvedBase, _ := filepath.EvalSymlinks(baseYaml)
	resolvedOv, _ := filepath.EvalSymlinks(overrideYaml)
	if savedOv != resolvedOv {
		t.Fatalf("expected saved file %q, got %q", resolvedOv, savedOv)
	}
	if len(deployFilesOv) != 2 || deployFilesOv[0] != resolvedBase || deployFilesOv[1] != resolvedOv {
		t.Fatalf("expected ordered deploy files [%q, %q], got %#v", resolvedBase, resolvedOv, deployFilesOv)
	}
	if gotBase, _ := os.ReadFile(baseYaml); string(gotBase) != origBase {
		t.Fatalf("base compose.yaml modified unexpectedly when saving override")
	}
	if gotOv, _ := os.ReadFile(overrideYaml); string(gotOv) != updatedOverride {
		t.Fatalf("compose.override.yaml not updated")
	}
}
