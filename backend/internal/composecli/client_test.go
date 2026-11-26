package composecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
