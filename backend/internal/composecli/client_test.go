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
	if err := c.DeleteProject(context.Background(), outsideCompose, StatusDraft); err == nil {
		t.Fatalf("expected DeleteProject outside APP_PATH to fail")
	}
	if _, err := os.Stat(outsideCompose); err != nil {
		t.Fatalf("expected outside compose directory to remain intact, stat err=%v", err)
	}

	// 3. Root APP_PATH itself cannot be deleted via compose.yaml inside root
	rootCompose := filepath.Join(appDir, "docker-compose.yaml")
	if _, _, err := ValidateComposeFilePath(rootCompose); err == nil {
		t.Fatalf("expected compose file directly under APP_PATH root to be rejected")
	}

	// 4. Valid project subdir succeeds
	saved, err := c.SaveNewProject(context.Background(), "valid-app", "services: {}\n", false)
	if err != nil {
		t.Fatalf("expected valid SaveNewProject to succeed, got %v", err)
	}
	if _, _, err := ValidateComposeFilePath(saved); err != nil {
		t.Fatalf("expected ValidateComposeFilePath(%q) to succeed, got %v", saved, err)
	}
}
