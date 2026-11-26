package composecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = appDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

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

func setupFakeDockerInPath(t *testing.T, scriptBody string) {
	t.Helper()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	content := "#!/bin/sh\n" + scriptBody + "\n"
	if err := os.WriteFile(dockerPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSaveProjectPlanFailClosedOnRuntimeQueryErrorAndHonorsDotEnvAndDraftSelection(t *testing.T) {
	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
	})

	ctx := context.Background()

	t.Run("docker compose ls non-zero exit fails before writing", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "ls-exit1-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		mainFile := filepath.Join(projDir, "compose.yaml")
		prodFile := filepath.Join(projDir, "prod.yaml")
		origMain := "services:\n  api:\n    image: nginx:1.25\n"
		origProd := "services:\n  worker:\n    image: alpine:3.19\n"
		if err := os.WriteFile(mainFile, []byte(origMain), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(prodFile, []byte(origProd), 0644); err != nil {
			t.Fatal(err)
		}

		setupFakeDockerInPath(t, `echo "daemon unreachable" >&2
exit 1`)
		c := &Client{}
		if _, err := c.SaveProjectPlanForDeploy(ctx, "ls-exit1-proj", mainFile, "services:\n  api:\n    image: nginx:1.28\n", true); err == nil {
			t.Fatalf("expected error when docker compose ls exits non-zero")
		}
		if got, _ := os.ReadFile(mainFile); string(got) != origMain {
			t.Fatalf("compose.yaml modified despite ls exit 1: %q", string(got))
		}
		if got, _ := os.ReadFile(prodFile); string(got) != origProd {
			t.Fatalf("prod.yaml modified despite ls exit 1: %q", string(got))
		}
	})

	t.Run("docker compose ls corrupt JSON fails before writing", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "ls-badjson-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		mainFile := filepath.Join(projDir, "compose.yaml")
		origMain := "services:\n  api:\n    image: nginx:1.25\n"
		if err := os.WriteFile(mainFile, []byte(origMain), 0644); err != nil {
			t.Fatal(err)
		}

		setupFakeDockerInPath(t, `printf '{corrupt-json'`)
		c := &Client{}
		if _, err := c.SaveProjectPlanForDeploy(ctx, "ls-badjson-proj", mainFile, "services:\n  api:\n    image: nginx:1.28\n", true); err == nil {
			t.Fatalf("expected error when docker compose ls outputs corrupt JSON")
		}
		if got, _ := os.ReadFile(mainFile); string(got) != origMain {
			t.Fatalf("compose.yaml modified despite corrupt JSON: %q", string(got))
		}
	})

	t.Run("docker compose ls returning empty array deploys draft normally", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "ls-empty-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		mainFile := filepath.Join(projDir, "compose.yaml")
		if err := os.WriteFile(mainFile, []byte("services:\n  api:\n    image: nginx:1.25\n"), 0644); err != nil {
			t.Fatal(err)
		}

		setupFakeDockerInPath(t, `printf '[]'`)
		c := &Client{}
		updated := "services:\n  api:\n    image: nginx:1.28\n"
		plan, err := c.SaveProjectPlanForDeploy(ctx, "ls-empty-proj", mainFile, updated, true)
		if err != nil {
			t.Fatalf("expected draft deploy to succeed when ls returns []: %v", err)
		}
		if plan.RuntimeProjectName != "" || len(plan.DeployFiles) != 1 || plan.DeployFiles[0] != mainFile {
			t.Fatalf("unexpected draft plan: %#v", plan)
		}
		if got, _ := os.ReadFile(mainFile); string(got) != updated {
			t.Fatalf("compose.yaml not updated: %q", string(got))
		}
	})

	t.Run("draft honors .env COMPOSE_FILE and COMPOSE_PATH_SEPARATOR", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "dotenv-draft-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		mainFile := filepath.Join(projDir, "compose.yaml")
		prodFile := filepath.Join(projDir, "prod.yaml")
		overrideFile := filepath.Join(projDir, "compose.override.yaml")
		envFile := filepath.Join(projDir, ".env")

		if err := os.WriteFile(mainFile, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(prodFile, []byte("services:\n  worker:\n    image: alpine:3.20\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(overrideFile, []byte("services:\n  ignored:\n    image: busybox:1.36\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(envFile, []byte("COMPOSE_PATH_SEPARATOR=;\nCOMPOSE_FILE=\"compose.yaml;prod.yaml\"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		c := &Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
				return nil, nil
			},
		}
		plan, err := c.SaveProjectPlanForDeploy(ctx, "dotenv-draft-proj", prodFile, "services:\n  worker:\n    image: alpine:3.21\n", true)
		if err != nil {
			t.Fatalf("expected editing member of .env COMPOSE_FILE to succeed: %v", err)
		}
		if plan.SavedFile != prodFile || plan.RuntimeProjectName != "" || len(plan.DeployFiles) != 2 || plan.DeployFiles[0] != mainFile || plan.DeployFiles[1] != prodFile {
			t.Fatalf("unexpected .env deploy plan: %#v", plan)
		}
		// overrideFile 不在 COMPOSE_FILE 中时，试图将其作为编辑目标必须在写盘前报错
		if _, err := c.SaveProjectPlanForDeploy(ctx, "dotenv-draft-proj", overrideFile, "services: {}\n", true); err == nil {
			t.Fatalf("expected conflict when editing file absent from .env COMPOSE_FILE")
		}
	})

	t.Run("multi-primary draft allows selecting either primary file explicitly in both orders", func(t *testing.T) {
		for _, order := range []string{"legacy-first", "compose-first"} {
			t.Run(order, func(t *testing.T) {
				projDir := filepath.Join(resolvedAppDir, "dual-primary-"+order)
				if err := os.MkdirAll(projDir, 0755); err != nil {
					t.Fatal(err)
				}
				composeYaml := filepath.Join(projDir, "compose.yaml")
				dockerComposeYml := filepath.Join(projDir, "docker-compose.yml")
				origCompose := "services:\n  a:\n    image: nginx:1.25\n"
				origLegacy := "services:\n  b:\n    image: busybox:1.36\n"
				if err := os.WriteFile(composeYaml, []byte(origCompose), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dockerComposeYml, []byte(origLegacy), 0644); err != nil {
					t.Fatal(err)
				}

				c := &Client{
					RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
						return nil, nil
					},
				}

				firstFile, secondFile := dockerComposeYml, composeYaml
				firstOtherContent, secondOtherContent := origCompose, origLegacy
				if order == "compose-first" {
					firstFile, secondFile = composeYaml, dockerComposeYml
					firstOtherContent, secondOtherContent = origLegacy, origCompose
				}

				// 选中第一份主文件（分别覆盖带 YAML 与仅启动解析）
				firstUpdated := "services:\n  picked:\n    image: alpine:3.20\n"
				plan1, err := c.SaveProjectPlanForDeploy(ctx, "dual-primary-"+order, firstFile, firstUpdated, true)
				if err != nil {
					t.Fatalf("first explicit primary save failed: %v", err)
				}
				if len(plan1.DeployFiles) == 0 || plan1.DeployFiles[0] != firstFile {
					t.Fatalf("expected first -f to target %q, got %#v", firstFile, plan1.DeployFiles)
				}
				otherFile := dockerComposeYml
				if firstFile == dockerComposeYml {
					otherFile = composeYaml
				}
				if gotOther, _ := os.ReadFile(otherFile); string(gotOther) != firstOtherContent {
					t.Fatalf("unselected primary %q modified: %q", otherFile, string(gotOther))
				}

				roPlan1, err := c.ResolveDeployPlan(ctx, "dual-primary-"+order, firstFile)
				if err != nil || len(roPlan1.DeployFiles) == 0 || roPlan1.DeployFiles[0] != firstFile {
					t.Fatalf("ResolveDeployPlan failed for %q: plan=%#v err=%v", firstFile, roPlan1, err)
				}

				// 接着选中第二份主文件
				secondUpdated := "services:\n  picked2:\n    image: alpine:3.21\n"
				plan2, err := c.SaveProjectPlanForDeploy(ctx, "dual-primary-"+order, secondFile, secondUpdated, true)
				if err != nil {
					t.Fatalf("second explicit primary save failed: %v", err)
				}
				if len(plan2.DeployFiles) == 0 || plan2.DeployFiles[0] != secondFile {
					t.Fatalf("expected first -f to target %q, got %#v", secondFile, plan2.DeployFiles)
				}
				if gotOther2, _ := os.ReadFile(firstFile); string(gotOther2) != firstUpdated {
					t.Fatalf("previously updated %q modified when saving %q", firstFile, secondFile)
				}
				_ = secondOtherContent
			})
		}
	})

	t.Run("SaveNewProject succeeds offline without querying runtime and refuses overwrite while SaveProjectPlanForDeploy fails closed", func(t *testing.T) {
		var listerCalls atomic.Int32
		offlineCli := &Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
				listerCalls.Add(1)
				return nil, errors.New("docker daemon offline")
			},
		}

		draftYaml := "services:\n  web:\n    image: nginx:1.27\n"
		savedFile, err := offlineCli.SaveNewProject(ctx, "offline-new-draft", draftYaml, false)
		if err != nil {
			t.Fatalf("expected SaveNewProject to succeed while Docker is offline: %v", err)
		}
		if listerCalls.Load() != 0 {
			t.Fatalf("expected 0 runtime lister calls during SaveNewProject, got %d", listerCalls.Load())
		}
		if got, err := os.ReadFile(savedFile); err != nil || string(got) != draftYaml {
			t.Fatalf("written yaml mismatch: got=%q err=%v", string(got), err)
		}

		// 同名重复新建拒绝覆盖，且不触发运行时查询
		if _, err := offlineCli.SaveNewProject(ctx, "offline-new-draft", "services:\n  other:\n    image: busybox:1.36\n", false); err == nil {
			t.Fatalf("expected duplicate SaveNewProject to refuse overwrite")
		}
		if listerCalls.Load() != 0 {
			t.Fatalf("expected 0 runtime lister calls on duplicate SaveNewProject, got %d", listerCalls.Load())
		}
		if got, _ := os.ReadFile(savedFile); string(got) != draftYaml {
			t.Fatalf("duplicate SaveNewProject overwrote original file: %q", string(got))
		}

		// 以同一查询错误执行 SaveProjectPlanForDeploy（保存并部署/重新部署），必须在写盘前失败并保持原文件完整
		if _, err := offlineCli.SaveProjectPlanForDeploy(ctx, "offline-new-draft", savedFile, "services:\n  mutated:\n    image: alpine:3.21\n", true); err == nil {
			t.Fatalf("expected SaveProjectPlanForDeploy to fail when runtime query errors")
		}
		if listerCalls.Load() != 1 {
			t.Fatalf("expected 1 runtime lister call during SaveProjectPlanForDeploy, got %d", listerCalls.Load())
		}
		if got, _ := os.ReadFile(savedFile); string(got) != draftYaml {
			t.Fatalf("SaveProjectPlanForDeploy modified file despite runtime error: %q", string(got))
		}
	})

	t.Run("missing file on unselected sibling runtime project does not block selected project in either record order", func(t *testing.T) {
		siteDir := filepath.Join(resolvedAppDir, "site")
		if err := os.MkdirAll(siteDir, 0755); err != nil {
			t.Fatal(err)
		}
		composeYaml := filepath.Join(siteDir, "compose.yaml")
		retiredYaml := filepath.Join(siteDir, "retired.yaml")
		origSiteYaml := "services:\n  live:\n    image: nginx:1.25\n"
		if err := os.WriteFile(composeYaml, []byte(origSiteYaml), 0644); err != nil {
			t.Fatal(err)
		}

		orders := []struct {
			name  string
			items []ListOutput
		}{
			{
				name: "live-then-retired",
				items: []ListOutput{
					{Name: "live", Status: "running(1)", ConfigFiles: composeYaml},
					{Name: "retired", Status: "exited(1)", ConfigFiles: retiredYaml},
				},
			},
			{
				name: "retired-then-live",
				items: []ListOutput{
					{Name: "retired", Status: "exited(1)", ConfigFiles: retiredYaml},
					{Name: "live", Status: "running(1)", ConfigFiles: composeYaml},
				},
			},
		}

		for _, tc := range orders {
			t.Run(tc.name, func(t *testing.T) {
				cli := &Client{
					RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
						return tc.items, nil
					},
				}
				roPlan, err := cli.ResolveDeployPlan(ctx, "site", composeYaml)
				if err != nil {
					t.Fatalf("ResolveDeployPlan failed (%s): %v", tc.name, err)
				}
				wantPrefix := []string{"-p", "live", "-f", composeYaml}
				if !reflect.DeepEqual(roPlan.ComposeArgsPrefix(), wantPrefix) {
					t.Fatalf("unexpected ResolveDeployPlan prefix: got=%#v want=%#v", roPlan.ComposeArgsPrefix(), wantPrefix)
				}

				updatedYaml := "services:\n  live:\n    image: nginx:1.28\n"
				savePlan, err := cli.SaveProjectPlanForDeploy(ctx, "site", composeYaml, updatedYaml, true)
				if err != nil {
					t.Fatalf("SaveProjectPlanForDeploy failed (%s): %v", tc.name, err)
				}
				if !reflect.DeepEqual(savePlan.ComposeArgsPrefix(), wantPrefix) {
					t.Fatalf("unexpected SaveProjectPlanForDeploy prefix: got=%#v want=%#v", savePlan.ComposeArgsPrefix(), wantPrefix)
				}
			})
		}

		// 若缺失文件被列为 live 自身的第二份配置，保存必须失败且不得写盘
		beforeFail := "services:\n  live:\n    image: nginx:1.29\n"
		if err := os.WriteFile(composeYaml, []byte(beforeFail), 0644); err != nil {
			t.Fatal(err)
		}
		brokenSelectedCli := &Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
				return []ListOutput{
					{Name: "live", Status: "running(1)", ConfigFiles: composeYaml + "," + retiredYaml},
				}, nil
			},
		}
		if _, err := brokenSelectedCli.SaveProjectPlanForDeploy(ctx, "site", composeYaml, "services:\n  live:\n    image: nginx:1.30\n", true); err == nil {
			t.Fatalf("expected SaveProjectPlanForDeploy to fail when selected project lists missing retired.yaml")
		}
		if got, _ := os.ReadFile(composeYaml); string(got) != beforeFail {
			t.Fatalf("compose.yaml modified despite missing member config file: %q", string(got))
		}
	})
}

func clearDotEnvTestProcessVars(t *testing.T) {
	t.Helper()
	keys := []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR", "COMPOSE_DISABLE_ENV_FILE", "COMPOSE_ENV_FILES", "LOCAL_STAGE", "STAGE", "NOTES"}
	for _, k := range keys {
		key := k
		if orig, had := os.LookupEnv(key); had {
			t.Cleanup(func() { _ = os.Setenv(key, orig) })
			_ = os.Unsetenv(key)
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(key) })
		}
	}
}

func TestDotEnvColonAssignment(t *testing.T) {
	clearDotEnvTestProcessVars(t)

	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	cases := []struct {
		name      string
		line      string
		want      string
		wantFiles []string
	}{
		{
			name:      "colon with space",
			line:      "COMPOSE_FILE: custom.yaml",
			want:      "custom.yaml",
			wantFiles: []string{"custom.yaml"},
		},
		{
			name:      "colon without space",
			line:      "COMPOSE_FILE:custom.yaml",
			want:      "custom.yaml",
			wantFiles: []string{"custom.yaml"},
		},
		{
			name:      "export colon with quoted equals in value",
			line:      `export COMPOSE_FILE: "stage=x.yaml"`,
			want:      "stage=x.yaml",
			wantFiles: []string{"stage=x.yaml"},
		},
		{
			name:      "colon with variable default expansion",
			line:      `COMPOSE_FILE: ${LOCAL_STAGE:-custom}.yaml`,
			want:      "custom.yaml",
			wantFiles: []string{"custom.yaml"},
		},
		{
			name:      "equals assignment preserves colon separator in value",
			line:      `COMPOSE_FILE=base.yaml:prod.yaml`,
			want:      "base.yaml:prod.yaml",
			wantFiles: []string{"base.yaml", "prod.yaml"},
		},
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return nil, nil
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COMPOSE_PATH_SEPARATOR", ":")
			projName := "colon-proj-" + string(rune('a'+i))
			projDir := filepath.Join(resolvedAppDir, projName)
			if err := os.MkdirAll(projDir, 0o755); err != nil {
				t.Fatal(err)
			}
			// 同时放入 compose.yaml（web）和目标自定义文件（worker），确认选中目标而非默认 compose.yaml
			if err := os.WriteFile(filepath.Join(projDir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			wantFullPaths := make([]string, 0, len(tc.wantFiles))
			for _, fname := range tc.wantFiles {
				p := filepath.Join(projDir, fname)
				if err := os.WriteFile(p, []byte("services:\n  worker:\n    image: alpine:3.20\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				wantFullPaths = append(wantFullPaths, p)
			}
			envPath := filepath.Join(projDir, ".env")
			if err := os.WriteFile(envPath, []byte(tc.line+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			envMap := make(map[string]string)
			if err := parseDotEnvFileInto(envPath, true, envMap); err != nil {
				t.Fatalf("parseDotEnvFileInto(%q) failed: %v", tc.line, err)
			}
			if envMap["COMPOSE_FILE"] != tc.want {
				t.Fatalf("line %q parsed %q, want %q", tc.line, envMap["COMPOSE_FILE"], tc.want)
			}

			plan, err := cli.ResolveDeployPlan(context.Background(), projName, "")
			if err != nil {
				t.Fatalf("ResolveDeployPlan failed: %v", err)
			}
			if !reflect.DeepEqual(plan.DeployFiles, wantFullPaths) {
				t.Fatalf("DeployFiles=%#v, want %#v", plan.DeployFiles, wantFullPaths)
			}
		})
	}
}

func TestResolveComposeTargetCustomRuntimeFile(t *testing.T) {
	ctx := context.Background()
	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	projectDir := filepath.Join(resolvedAppDir, "site")
	otherDir := filepath.Join(resolvedAppDir, "other")
	extDir := filepath.Join(tmpRoot, "external")
	for _, d := range []string{projectDir, otherDir, extDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	prodFile := filepath.Join(projectDir, "prod.yaml")
	overrideFile := filepath.Join(projectDir, "override.yaml")
	otherCustomFile := filepath.Join(projectDir, "extra.yaml")
	stdComposeFile := filepath.Join(projectDir, "compose.yaml")
	otherDirProdFile := filepath.Join(otherDir, "prod.yaml")
	extFile := filepath.Join(extDir, "prod.yaml")
	txtFile := filepath.Join(projectDir, "notes.txt")
	symFile := filepath.Join(projectDir, "sym.yaml")
	for _, f := range []string{prodFile, overrideFile, otherCustomFile, stdComposeFile, otherDirProdFile, extFile} {
		if err := os.WriteFile(f, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(txtFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(prodFile, symFile); err != nil {
		t.Fatal(err)
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return []ListOutput{
				{
					Name:        "blue",
					Status:      "running(1)",
					ConfigFiles: prodFile + "," + overrideFile,
				},
				{
					Name:        "ext-stack",
					Status:      "running(1)",
					ConfigFiles: extFile,
				},
			}, nil
		},
	}

	wantPrefix := []string{"-p", "blue", "-f", prodFile, "-f", overrideFile}
	// 运行时名称或列表合并时保留的目录名均应匹配成功
	for _, reqName := range []string{"blue", "site"} {
		dir, prefix, err := cli.ResolveComposeTarget(ctx, reqName, prodFile)
		if err != nil || dir != projectDir || !slices.Equal(prefix, wantPrefix) {
			t.Fatalf("ResolveComposeTarget(%q) = %q, %q, %v (want dir=%q prefix=%q)", reqName, dir, prefix, err, projectDir, wantPrefix)
		}
	}

	// StopProject 应在同一目录与完整前缀参数之后追加 stop
	logDir := t.TempDir()
	cmdLog := filepath.Join(logDir, "docker-args.log")
	pwdLog := filepath.Join(logDir, "docker-pwd.log")
	setupFakeDockerInPath(t, "pwd > "+pwdLog+"\nprintf '%s\\n' \"$@\" > "+cmdLog)
	if err := cli.StopProject(ctx, "blue", prodFile); err != nil {
		t.Fatalf("StopProject failed: %v", err)
	}
	gotPwd, _ := os.ReadFile(pwdLog)
	if strings.TrimSpace(string(gotPwd)) != projectDir {
		t.Fatalf("StopProject pwd=%q, want %q", strings.TrimSpace(string(gotPwd)), projectDir)
	}
	gotArgsRaw, _ := os.ReadFile(cmdLog)
	gotArgs := strings.Split(strings.TrimSpace(string(gotArgsRaw)), "\n")
	wantStopArgs := append(append([]string{"compose"}, wantPrefix...), "stop")
	if !slices.Equal(gotArgs, wantStopArgs) {
		t.Fatalf("StopProject args=%#v, want %#v", gotArgs, wantStopArgs)
	}

	// 对照用例 1：标准受管文件直接返回 (projectDir, nil, nil)
	if dir, prefix, err := cli.ResolveComposeTarget(ctx, "site", stdComposeFile); err != nil || dir != projectDir || len(prefix) != 0 {
		t.Fatalf("standard composeFile = (%q, %#v, %v), want (%q, nil, nil)", dir, prefix, err, projectDir)
	}
	// 对照用例 2：外部运行时项目返回 ("", ["-p", "ext-stack"], nil)
	if dir, prefix, err := cli.ResolveComposeTarget(ctx, "ext-stack", extFile); err != nil || dir != "" || !slices.Equal(prefix, []string{"-p", "ext-stack"}) {
		t.Fatalf("external runtime project = (%q, %#v, %v)", dir, prefix, err)
	}
	// 对照用例 3：ListProjects 合并后的逗号分隔 ConfigFiles 继续回退到 -p blue
	if dir, prefix, err := cli.ResolveComposeTarget(ctx, "blue", prodFile+","+overrideFile); err != nil || dir != "" || !slices.Equal(prefix, []string{"-p", "blue"}) {
		t.Fatalf("comma-separated ConfigFiles = (%q, %#v, %v)", dir, prefix, err)
	}
	// 对照用例 4：StatusDraft 删除仍走 ValidateComposeFilePath 并拒绝自定义文件名
	if err := cli.DeleteProject(ctx, "site", prodFile, StatusDraft); err == nil {
		t.Fatalf("expected StatusDraft DeleteProject to reject custom filename %q", prodFile)
	}

	// 负例验证
	t.Run("negative cases", func(t *testing.T) {
		if _, _, err := cli.ResolveComposeTarget(ctx, "other-name", prodFile); err == nil {
			t.Fatal("expected mismatched project name to fail")
		}
		if _, _, err := cli.ResolveComposeTarget(ctx, "blue", otherCustomFile); err == nil {
			t.Fatal("expected unreferenced custom file extra.yaml in same dir to fail")
		}
		if _, _, err := cli.ResolveComposeTarget(ctx, "blue", otherDirProdFile); err == nil {
			t.Fatal("expected same project name in different directory to fail")
		}
		if _, _, err := cli.ResolveComposeTarget(ctx, "blue", symFile); !errors.Is(err, ErrSymlinkPath) {
			t.Fatalf("expected ErrSymlinkPath for %q, got %v", symFile, err)
		}
		traversalPath := projectDir + string(filepath.Separator) + ".." + string(filepath.Separator) + "site" + string(filepath.Separator) + "prod.yaml"
		if _, _, err := cli.ResolveComposeTarget(ctx, "blue", traversalPath); !errors.Is(err, ErrTraversalPath) {
			t.Fatalf("expected ErrTraversalPath, got %v", err)
		}
		if _, _, err := cli.ResolveComposeTarget(ctx, "blue", txtFile); err == nil {
			t.Fatal("expected non-yaml custom filename to fail")
		}

		emptyCli := &Client{RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) { return nil, nil }}
		if _, _, err := emptyCli.ResolveComposeTarget(ctx, "blue", prodFile); err == nil {
			t.Fatal("expected empty runtime list to reject custom managed file")
		}

		errCli := &Client{RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) { return nil, errors.New("daemon offline") }}
		if _, _, err := errCli.ResolveComposeTarget(ctx, "blue", prodFile); err == nil {
			t.Fatal("expected runtime query error to fail")
		}

		ambiguousCli := &Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
				return []ListOutput{
					{Name: "blue", Status: "running(1)", ConfigFiles: prodFile},
					{Name: "green", Status: "running(1)", ConfigFiles: prodFile},
				}, nil
			},
		}
		if _, _, err := ambiguousCli.ResolveComposeTarget(ctx, "site", prodFile); err == nil {
			t.Fatal("expected ambiguous multiple runtime matches for same file to fail")
		}
	})
}

func TestSaveNewProjectCreatesMissingAppRoot(t *testing.T) {
	ctx := context.Background()

	t.Run("creates missing nested APP_PATH offline and rejects duplicate or invalid names", func(t *testing.T) {
		baseDir := t.TempDir()
		missingRoot := filepath.Join(baseDir, "data", "compose")

		origEnv := conf.EnvCfg
		testEnv := *origEnv
		testEnv.APP_PATH = missingRoot
		conf.EnvCfg = &testEnv
		t.Cleanup(func() { conf.EnvCfg = origEnv })

		var listerCalls atomic.Int32
		cli := &Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
				listerCalls.Add(1)
				return nil, errors.New("docker daemon offline")
			},
		}

		// 非法项目名失败时不得创建缺失的 APP_PATH
		if _, err := cli.SaveNewProject(ctx, "../bad", "services: {}\n", false); err == nil {
			t.Fatal("expected invalid project name to fail")
		}
		if _, statErr := os.Stat(missingRoot); !os.IsNotExist(statErr) {
			t.Fatalf("missing APP_PATH must not be created on invalid project name, stat err=%v", statErr)
		}

		yaml1 := "services:\n  demo:\n    image: nginx:1.27\n"
		saved, err := cli.SaveNewProject(ctx, "demo", yaml1, false)
		if err != nil {
			t.Fatalf("expected SaveNewProject to initialize missing APP_PATH, got %v", err)
		}
		if listerCalls.Load() != 0 {
			t.Fatalf("expected 0 runtime lister calls, got %d", listerCalls.Load())
		}
		resolvedRoot, err := filepath.EvalSymlinks(missingRoot)
		if err != nil {
			t.Fatal(err)
		}
		wantFile := filepath.Join(resolvedRoot, "demo", "docker-compose.yaml")
		if saved != wantFile {
			t.Fatalf("saved path=%q, want %q", saved, wantFile)
		}
		if got, err := os.ReadFile(saved); err != nil || string(got) != yaml1 {
			t.Fatalf("saved content=%q err=%v, want %q", string(got), err, yaml1)
		}

		// 第二次同名保存应拒绝覆盖且保持首次内容不变
		if _, err := cli.SaveNewProject(ctx, "demo", "services:\n  demo:\n    image: alpine:3.20\n", false); err == nil {
			t.Fatal("expected duplicate SaveNewProject to fail")
		}
		if got, _ := os.ReadFile(saved); string(got) != yaml1 {
			t.Fatalf("duplicate SaveNewProject overwrote file: %q", string(got))
		}
		if listerCalls.Load() != 0 {
			t.Fatalf("expected 0 runtime lister calls after duplicate save, got %d", listerCalls.Load())
		}

		// 验证运行时查询失败仍阻止已有草稿的 SaveProjectPlanForDeploy
		if _, err := cli.SaveProjectPlanForDeploy(ctx, "demo", saved, "services:\n  demo:\n    image: busybox:1.36\n", true); err == nil {
			t.Fatal("expected SaveProjectPlanForDeploy to fail when runtime query errors")
		}
		if got, _ := os.ReadFile(saved); string(got) != yaml1 {
			t.Fatalf("SaveProjectPlanForDeploy modified file despite runtime error: %q", string(got))
		}
	})

	t.Run("symlinked APP_PATH with missing leaf resolves cleanly", func(t *testing.T) {
		baseDir := t.TempDir()
		realParent := filepath.Join(baseDir, "real")
		linkParent := filepath.Join(baseDir, "link")
		if err := os.MkdirAll(realParent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realParent, linkParent); err != nil {
			t.Fatal(err)
		}

		origEnv := conf.EnvCfg
		testEnv := *origEnv
		testEnv.APP_PATH = filepath.Join(linkParent, "compose-root")
		conf.EnvCfg = &testEnv
		t.Cleanup(func() { conf.EnvCfg = origEnv })

		cli := &Client{}
		saved, err := cli.SaveNewProject(ctx, "sym-demo", "services:\n  app:\n    image: alpine:3.20\n", false)
		if err != nil {
			t.Fatalf("expected symlinked APP_PATH creation to succeed: %v", err)
		}
		resolvedParent, _ := filepath.EvalSymlinks(realParent)
		wantPrefix := filepath.Join(resolvedParent, "compose-root", "sym-demo", "docker-compose.yaml")
		if saved != wantPrefix {
			t.Fatalf("saved path=%q, want canonical %q", saved, wantPrefix)
		}
	})

	t.Run("regular file APP_PATH fails with path error", func(t *testing.T) {
		baseDir := t.TempDir()
		fileRoot := filepath.Join(baseDir, "not-a-dir")
		if err := os.WriteFile(fileRoot, []byte("blocker"), 0o644); err != nil {
			t.Fatal(err)
		}

		origEnv := conf.EnvCfg
		testEnv := *origEnv
		testEnv.APP_PATH = fileRoot
		conf.EnvCfg = &testEnv
		t.Cleanup(func() { conf.EnvCfg = origEnv })

		cli := &Client{}
		_, err := cli.SaveNewProject(ctx, "demo", "services: {}\n", false)
		if err == nil || !strings.Contains(err.Error(), "not-a-dir") {
			t.Fatalf("expected error mentioning %q, got %v", fileRoot, err)
		}
	})

	t.Run("concurrent duplicate SaveNewProject yields exact one winner", func(t *testing.T) {
		baseDir := t.TempDir()
		missingRoot := filepath.Join(baseDir, "concurrent-root")

		origEnv := conf.EnvCfg
		testEnv := *origEnv
		testEnv.APP_PATH = missingRoot
		conf.EnvCfg = &testEnv
		t.Cleanup(func() { conf.EnvCfg = origEnv })

		cli := &Client{}
		yamlA := "services:\n  a:\n    image: nginx:1.25\n"
		yamlB := "services:\n  b:\n    image: nginx:1.27\n"

		type res struct {
			path string
			yaml string
			err  error
		}
		start := make(chan struct{})
		results := make(chan res, 2)
		var wg sync.WaitGroup
		for _, y := range []string{yamlA, yamlB} {
			payload := y
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				p, err := cli.SaveNewProject(ctx, "race-demo", payload, false)
				results <- res{path: p, yaml: payload, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		var winners, losers int
		var winnerRes res
		for r := range results {
			if r.err == nil {
				winners++
				winnerRes = r
			} else {
				losers++
			}
		}
		if winners != 1 || losers != 1 {
			t.Fatalf("expected 1 winner and 1 loser, got winners=%d losers=%d", winners, losers)
		}
		if got, err := os.ReadFile(winnerRes.path); err != nil || string(got) != winnerRes.yaml {
			t.Fatalf("final file content=%q err=%v, want %q", string(got), err, winnerRes.yaml)
		}
	})
}

func TestDotEnvMultilineAssignment(t *testing.T) {
	clearDotEnvTestProcessVars(t)
	ctx := context.Background()

	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	projDir := filepath.Join(resolvedAppDir, "multi-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(projDir, "compose.yaml")
	prodFile := filepath.Join(projDir, "prod.yaml")
	envFile := filepath.Join(projDir, ".env")
	if err := os.WriteFile(composeFile, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prodFile, []byte("services:\n  worker:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return nil, nil
		},
	}

	// 1. 多行单引号 NOTES 内部包含 "COMPOSE_FILE: prod.yaml"，不应额外定义 COMPOSE_FILE
	multilineNotes := "NOTES='example\nCOMPOSE_FILE: prod.yaml\nend'\n"
	if err := os.WriteFile(envFile, []byte(multilineNotes), 0o644); err != nil {
		t.Fatal(err)
	}
	envMap := make(map[string]string)
	if err := parseDotEnvFileInto(envFile, true, envMap); err != nil {
		t.Fatalf("parse multiline single-quote .env failed: %v", err)
	}
	wantNotes := "example\nCOMPOSE_FILE: prod.yaml\nend"
	if envMap["NOTES"] != wantNotes {
		t.Fatalf("NOTES=%q, want %q", envMap["NOTES"], wantNotes)
	}
	if val, exists := envMap["COMPOSE_FILE"]; exists {
		t.Fatalf("expected COMPOSE_FILE absent inside multiline NOTES, got %q", val)
	}
	plan1, err := cli.ResolveDeployPlan(ctx, "multi-proj", "")
	if err != nil {
		t.Fatalf("ResolveDeployPlan failed: %v", err)
	}
	if !slices.Equal(plan1.DeployFiles, []string{composeFile}) {
		t.Fatalf("expected [compose.yaml] when COMPOSE_FILE is inside multiline NOTES, got %#v", plan1.DeployFiles)
	}

	// 2. 在同一多行 NOTES 后追加真实顶层赋值 COMPOSE_FILE: prod.yaml，此时选中 prod.yaml
	withTopLevel := multilineNotes + "COMPOSE_FILE: prod.yaml\n"
	if err := os.WriteFile(envFile, []byte(withTopLevel), 0o644); err != nil {
		t.Fatal(err)
	}
	envMap2 := make(map[string]string)
	if err := parseDotEnvFileInto(envFile, true, envMap2); err != nil {
		t.Fatalf("parse multiline + top-level .env failed: %v", err)
	}
	if envMap2["COMPOSE_FILE"] != "prod.yaml" {
		t.Fatalf("COMPOSE_FILE=%q, want prod.yaml", envMap2["COMPOSE_FILE"])
	}
	plan2, err := cli.ResolveDeployPlan(ctx, "multi-proj", "")
	if err != nil {
		t.Fatalf("ResolveDeployPlan failed: %v", err)
	}
	if !slices.Equal(plan2.DeployFiles, []string{prodFile}) {
		t.Fatalf("expected [prod.yaml], got %#v", plan2.DeployFiles)
	}

	// 3. 多行双引号、UTF-8 BOM / CRLF、${STAGE:-prod} 展开与含等号文件名
	stageEqualsYaml := filepath.Join(projDir, "stage=x.yaml")
	if err := os.WriteFile(stageEqualsYaml, []byte("services:\n  stage_svc:\n    image: busybox:1.36\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bomCRLF := "\xef\xbb\xbfNOTES=\"line1\r\nline2\"\r\nDEFAULTED: ${STAGE:-prod}.yaml\r\nCOMPOSE_FILE: \"stage=x.yaml\"\r\n"
	if err := os.WriteFile(envFile, []byte(bomCRLF), 0o644); err != nil {
		t.Fatal(err)
	}
	envMap3 := make(map[string]string)
	if err := parseDotEnvFileInto(envFile, true, envMap3); err != nil {
		t.Fatalf("parse BOM/CRLF multiline double-quote failed: %v", err)
	}
	if envMap3["NOTES"] != "line1\r\nline2" && envMap3["NOTES"] != "line1\nline2" {
		t.Fatalf("unexpected BOM/CRLF NOTES=%q", envMap3["NOTES"])
	}
	if envMap3["DEFAULTED"] != "prod.yaml" {
		t.Fatalf("expected ${STAGE:-prod}.yaml == prod.yaml, got %q", envMap3["DEFAULTED"])
	}
	if envMap3["COMPOSE_FILE"] != "stage=x.yaml" {
		t.Fatalf("expected stage=x.yaml, got %q", envMap3["COMPOSE_FILE"])
	}

	// 4. 第二份 env 文件重新赋值后同文件内再次引用该变量
	firstEnv := filepath.Join(projDir, "first.env")
	secondEnv := filepath.Join(projDir, "second.env")
	if err := os.WriteFile(firstEnv, []byte("STAGE=prod\nKEEP=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondEnv, []byte("STAGE=override\nCOMPOSE_FILE=${STAGE}.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overrideTarget := filepath.Join(projDir, "override.yaml")
	if err := os.WriteFile(overrideTarget, []byte("services:\n  ov:\n    image: busybox:1.36\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chainedMap := make(map[string]string)
	if err := parseDotEnvFileInto(firstEnv, true, chainedMap); err != nil {
		t.Fatalf("parse first.env failed: %v", err)
	}
	if err := parseDotEnvFileInto(secondEnv, true, chainedMap); err != nil {
		t.Fatalf("parse second.env failed: %v", err)
	}
	if chainedMap["COMPOSE_FILE"] != "override.yaml" || chainedMap["KEEP"] != "1" {
		t.Fatalf("chained env reassignment mismatch: %#v", chainedMap)
	}
	t.Run("COMPOSE_ENV_FILES chained reassignment selects override.yaml", func(t *testing.T) {
		t.Setenv("COMPOSE_ENV_FILES", "first.env,second.env")
		chainedPlan, perr := cli.ResolveDeployPlan(ctx, "multi-proj", "")
		if perr != nil {
			t.Fatalf("ResolveDeployPlan with chained COMPOSE_ENV_FILES failed: %v", perr)
		}
		if !slices.Equal(chainedPlan.DeployFiles, []string{overrideTarget}) {
			t.Fatalf("expected [override.yaml], got %#v", chainedPlan.DeployFiles)
		}
	})

	// 5. 未闭合引号必须返回错误，且不得部分修改调用方 envMap
	badEnv := filepath.Join(projDir, "bad-quote.env")
	if err := os.WriteFile(badEnv, []byte("GOOD_KEY=after\nNOTES=\"unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	preservedMap := map[string]string{"EXISTING": "preserve-me"}
	if err := parseDotEnvFileInto(badEnv, true, preservedMap); err == nil {
		t.Fatal("expected unterminated quote error")
	}
	if len(preservedMap) != 1 || preservedMap["EXISTING"] != "preserve-me" {
		t.Fatalf("expected preservedMap untouched on parse failure, got %#v", preservedMap)
	}
}

func TestResolveManagedCardPlan(t *testing.T) {
	clearDotEnvTestProcessVars(t)
	ctx := context.Background()

	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	extDir := filepath.Join(tmpRoot, "external")
	for _, d := range []string{appDir, extDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	// 1. 标准草稿（含 compose.override.yml）
	stdDir := filepath.Join(resolvedAppDir, "std-draft")
	otherDir := filepath.Join(resolvedAppDir, "other-proj")
	customDraftDir := filepath.Join(resolvedAppDir, "custom-draft")
	siteDir := filepath.Join(resolvedAppDir, "site")
	for _, d := range []string{stdDir, otherDir, customDraftDir, siteDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stdMain := filepath.Join(stdDir, "compose.yaml")
	stdOv := filepath.Join(stdDir, "compose.override.yml")
	otherFile := filepath.Join(otherDir, "compose.yaml")
	customFile := filepath.Join(customDraftDir, "prod.yaml")
	customEnv := filepath.Join(customDraftDir, ".env")
	siteProd := filepath.Join(siteDir, "prod.yaml")
	siteOv := filepath.Join(siteDir, "override.yaml")
	extFile := filepath.Join(extDir, "compose.yaml")
	symFile := filepath.Join(siteDir, "sym.yaml")

	for _, f := range []string{stdMain, stdOv, otherFile, customFile, siteProd, siteOv, extFile} {
		if err := os.WriteFile(f, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(customEnv, []byte("COMPOSE_FILE: prod.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(siteProd, symFile); err != nil {
		t.Fatal(err)
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return []ListOutput{
				{Name: "blue", Status: "running(1)", ConfigFiles: siteProd + "," + siteOv},
				{Name: "same-name", Status: "running(1)", ConfigFiles: otherFile},
				{Name: "ext", Status: "running(1)", ConfigFiles: extFile},
			}, nil
		},
	}

	// 标准草稿
	stdPlan, err := cli.ResolveManagedCardPlan(ctx, "std-draft", stdMain)
	if err != nil {
		t.Fatalf("standard draft ResolveManagedCardPlan failed: %v", err)
	}
	if stdPlan.ProjectDir != stdDir || stdPlan.RuntimeProjectName != "" || !slices.Equal(stdPlan.DeployFiles, []string{stdMain, stdOv}) {
		t.Fatalf("unexpected standard draft plan: %#v", stdPlan)
	}

	// .env 指定的自定义草稿
	customPlan, err := cli.ResolveManagedCardPlan(ctx, "custom-draft", customFile)
	if err != nil {
		t.Fatalf("dotenv custom draft ResolveManagedCardPlan failed: %v", err)
	}
	if customPlan.ProjectDir != customDraftDir || customPlan.RuntimeProjectName != "" || !slices.Equal(customPlan.DeployFiles, []string{customFile}) {
		t.Fatalf("unexpected custom draft plan: %#v", customPlan)
	}

	// 运行时受管项目（单文件 & 顺序一致的逗号列表，均支持运行时名与目录名）
	for _, reqName := range []string{"blue", "site"} {
		for _, filesArg := range []string{siteProd, siteProd + "," + siteOv} {
			plan, err := cli.ResolveManagedCardPlan(ctx, reqName, filesArg)
			if err != nil {
				t.Fatalf("runtime managed plan (%s, %s) failed: %v", reqName, filesArg, err)
			}
			wantPrefix := []string{"-p", "blue", "-f", siteProd, "-f", siteOv}
			if plan.ProjectDir != siteDir || !slices.Equal(plan.ComposeArgsPrefix(), wantPrefix) {
				t.Fatalf("unexpected runtime plan (%s, %s): dir=%q prefix=%#v", reqName, filesArg, plan.ProjectDir, plan.ComposeArgsPrefix())
			}
		}
	}

	// 不同目录同名项目不得误匹配
	if _, err := cli.ResolveManagedCardPlan(ctx, "same-name", stdMain); err == nil {
		t.Fatal("expected same project name in different directory to fail")
	}
	// 乱序多文件与混入其他项目文件必须失败
	if _, err := cli.ResolveManagedCardPlan(ctx, "blue", siteOv+","+siteProd); err == nil {
		t.Fatal("expected out-of-order multi-file list to fail")
	}
	if _, err := cli.ResolveManagedCardPlan(ctx, "blue", siteProd+","+otherFile); err == nil {
		t.Fatal("expected cross-project mixed multi-file list to fail")
	}
	// 符号链接、目录穿越、外部项目必须失败
	if _, err := cli.ResolveManagedCardPlan(ctx, "blue", symFile); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("expected ErrSymlinkPath for %q, got %v", symFile, err)
	}
	traversalPath := siteDir + string(filepath.Separator) + ".." + string(filepath.Separator) + "site" + string(filepath.Separator) + "prod.yaml"
	if _, err := cli.ResolveManagedCardPlan(ctx, "blue", traversalPath); !errors.Is(err, ErrTraversalPath) {
		t.Fatalf("expected ErrTraversalPath, got %v", err)
	}
	if _, err := cli.ResolveManagedCardPlan(ctx, "ext", extFile); err == nil {
		t.Fatal("expected external unmanaged project to fail")
	}
	// 运行时查询失败必须中止
	errCli := &Client{RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) { return nil, errors.New("offline") }}
	if _, err := errCli.ResolveManagedCardPlan(ctx, "std-draft", stdMain); err == nil {
		t.Fatal("expected runtime query error to fail")
	}
}

func TestManagedDraftCardUsesEnvFiles(t *testing.T) {
	clearDotEnvTestProcessVars(t)
	ctx := context.Background()

	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	// 1. site 同时拥有 compose.yaml、prod.yaml 与声明 COMPOSE_FILE=prod.yaml 的 .env：
	// 从 ListProjects 返回的卡片直接调用 ResolveManagedCardPlan，应采用 prod.yaml
	siteDir := filepath.Join(resolvedAppDir, "site")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(siteDir, "compose.yaml")
	prodFile := filepath.Join(siteDir, "prod.yaml")
	otherFile := filepath.Join(siteDir, "other.yaml")
	for _, item := range []struct {
		path    string
		content string
	}{
		{path: composeFile, content: "services:\n  web:\n    image: nginx:1.25\n"},
		{path: prodFile, content: "services:\n  production:\n    image: nginx:1.25\n"},
		{path: otherFile, content: "services:\n  other:\n    image: alpine:3.20\n"},
	} {
		if err := os.WriteFile(item.path, []byte(item.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(siteDir, ".env"), []byte("COMPOSE_FILE=prod.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return nil, nil
		},
	}

	cards, err := cli.ListProjects(ctx)
	if err != nil || len(cards) != 1 {
		t.Fatalf("ListProjects() = %v, %v", cards, err)
	}
	if cards[0].Name != "site" || cards[0].ComposeFile != composeFile {
		t.Fatalf("unexpected draft card from ListProjects: %#v (want ComposeFile=%q)", cards[0], composeFile)
	}
	plan, err := cli.ResolveManagedCardPlan(ctx, cards[0].Name, cards[0].ComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ComposeArgsPrefix(); !slices.Equal(got, []string{"-f", prodFile}) {
		t.Fatalf("ComposeArgsPrefix() = %v, want %v", got, []string{"-f", prodFile})
	}

	// 传入不在 COMPOSE_FILE 中的自定义非标准文件名，依然必须报成员冲突
	if _, err := cli.ResolveManagedCardPlan(ctx, "site", otherFile); err == nil {
		t.Fatal("expected conflict when custom non-primary file is absent from .env COMPOSE_FILE")
	}

	// 2. 无 .env COMPOSE_FILE 时，双标准主文件目录按各卡片选中的主文件部署
	dualDir := filepath.Join(resolvedAppDir, "dual-no-env")
	if err := os.MkdirAll(dualDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dualCompose := filepath.Join(dualDir, "compose.yaml")
	dualLegacy := filepath.Join(dualDir, "docker-compose.yml")
	for _, f := range []string{dualCompose, dualLegacy} {
		if err := os.WriteFile(f, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, selected := range []string{dualCompose, dualLegacy} {
		dualPlan, derr := cli.ResolveManagedCardPlan(ctx, "dual-no-env", selected)
		if derr != nil {
			t.Fatalf("dual-no-env ResolveManagedCardPlan(%q) failed: %v", selected, derr)
		}
		if !slices.Equal(dualPlan.ComposeArgsPrefix(), []string{"-f", selected}) {
			t.Fatalf("dual-no-env prefix=%v, want [-f %s]", dualPlan.ComposeArgsPrefix(), selected)
		}
	}
}

func TestDraftComposeFilePresence(t *testing.T) {
	clearDotEnvTestProcessVars(t)
	ctx := context.Background()

	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedAppDir, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		t.Fatal(err)
	}

	origEnv := conf.EnvCfg
	testEnv := *origEnv
	testEnv.APP_PATH = resolvedAppDir
	conf.EnvCfg = &testEnv
	t.Cleanup(func() { conf.EnvCfg = origEnv })

	siteDir := filepath.Join(resolvedAppDir, "site")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(siteDir, "compose.yaml")
	prodFile := filepath.Join(siteDir, "prod.yaml")
	envFile := filepath.Join(siteDir, ".env")
	origCompose := "services:\n  web:\n    image: nginx:1.25\n"
	origProd := "services:\n  production:\n    image: nginx:1.26\n"
	if err := os.WriteFile(composeFile, []byte(origCompose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prodFile, []byte(origProd), 0o644); err != nil {
		t.Fatal(err)
	}

	cli := &Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]ListOutput, error) {
			return nil, nil
		},
	}

	assertFilesUntouched := func(t *testing.T) {
		t.Helper()
		if got, rerr := os.ReadFile(composeFile); rerr != nil || string(got) != origCompose {
			t.Fatalf("compose.yaml modified: err=%v got=%q", rerr, string(got))
		}
		if got, rerr := os.ReadFile(prodFile); rerr != nil || string(got) != origProd {
			t.Fatalf("prod.yaml modified: err=%v got=%q", rerr, string(got))
		}
	}

	t.Run("process unset and dotenv COMPOSE_FILE empty fails closed", func(t *testing.T) {
		_ = os.Unsetenv("COMPOSE_FILE")
		if err := os.WriteFile(envFile, []byte("COMPOSE_FILE=\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if plan, perr := cli.ResolveDeployPlan(ctx, "site", ""); perr == nil || len(plan.DeployFiles) != 0 {
			t.Fatalf("ResolveDeployPlan should fail with empty DeployFiles, got plan=%#v err=%v", plan, perr)
		}
		if plan, perr := cli.ResolveManagedCardPlan(ctx, "site", composeFile); perr == nil || len(plan.DeployFiles) != 0 {
			t.Fatalf("ResolveManagedCardPlan should fail with empty DeployFiles, got plan=%#v err=%v", plan, perr)
		}
		for _, target := range []string{"", composeFile} {
			if _, serr := cli.SaveProjectPlanForDeploy(ctx, "site", target, "services:\n  mutated:\n    image: busybox\n", true); serr == nil {
				t.Fatalf("SaveProjectPlanForDeploy(%q) should fail when .env COMPOSE_FILE is empty", target)
			}
		}
		assertFilesUntouched(t)
	})

	t.Run("process empty COMPOSE_FILE overrides dotenv and fails closed", func(t *testing.T) {
		if err := os.WriteFile(envFile, []byte("COMPOSE_FILE=prod.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("COMPOSE_FILE", "")

		if plan, perr := cli.ResolveDeployPlan(ctx, "site", ""); perr == nil || len(plan.DeployFiles) != 0 {
			t.Fatalf("ResolveDeployPlan should fail when process COMPOSE_FILE=\"\", got plan=%#v err=%v", plan, perr)
		}
		if plan, perr := cli.ResolveManagedCardPlan(ctx, "site", composeFile); perr == nil || len(plan.DeployFiles) != 0 {
			t.Fatalf("ResolveManagedCardPlan should fail when process COMPOSE_FILE=\"\", got plan=%#v err=%v", plan, perr)
		}
		for _, target := range []string{"", prodFile} {
			if _, serr := cli.SaveProjectPlanForDeploy(ctx, "site", target, "services:\n  mutated:\n    image: busybox\n", true); serr == nil {
				t.Fatalf("SaveProjectPlanForDeploy(%q) should fail when process COMPOSE_FILE=\"\"", target)
			}
		}
		assertFilesUntouched(t)
	})

	t.Run("positive neither set selects primary compose.yaml", func(t *testing.T) {
		_ = os.Unsetenv("COMPOSE_FILE")
		if err := os.RemoveAll(envFile); err != nil {
			t.Fatal(err)
		}

		plan, perr := cli.ResolveDeployPlan(ctx, "site", "")
		if perr != nil || !slices.Equal(plan.DeployFiles, []string{composeFile}) {
			t.Fatalf("expected [%s], got plan=%#v err=%v", composeFile, plan, perr)
		}
	})

	t.Run("positive process unset and dotenv valid selects prod.yaml", func(t *testing.T) {
		_ = os.Unsetenv("COMPOSE_FILE")
		if err := os.WriteFile(envFile, []byte("COMPOSE_FILE=prod.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		plan, perr := cli.ResolveDeployPlan(ctx, "site", "")
		if perr != nil || !slices.Equal(plan.DeployFiles, []string{prodFile}) {
			t.Fatalf("ResolveDeployPlan expected [%s], got plan=%#v err=%v", prodFile, plan, perr)
		}
		cardPlan, cerr := cli.ResolveManagedCardPlan(ctx, "site", composeFile)
		if cerr != nil || !slices.Equal(cardPlan.DeployFiles, []string{prodFile}) {
			t.Fatalf("ResolveManagedCardPlan expected [%s], got plan=%#v err=%v", prodFile, cardPlan, cerr)
		}
	})
}
