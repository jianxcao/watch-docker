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
	keys := []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR", "COMPOSE_DISABLE_ENV_FILE", "COMPOSE_ENV_FILES", "LOCAL_STAGE"}
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
