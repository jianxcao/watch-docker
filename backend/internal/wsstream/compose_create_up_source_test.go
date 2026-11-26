package wsstream

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

func drainCreateUpStream(t *testing.T, src *ComposeCreateUpSource) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reader, err := src.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer reader.Close()

	var out strings.Builder
	for {
		chunk, rerr := reader.Read(ctx)
		if len(chunk) > 0 {
			out.Write(chunk)
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			t.Fatalf("Read failed: %v", rerr)
		}
	}
	return out.String()
}

func okStreamExecutor() (func(context.Context, composecli.ExecDockerComposeStreamOptions) *composecli.ExecDockerComposeStreamResult, *[]composecli.ExecDockerComposeStreamOptions) {
	var mu sync.Mutex
	var calls []composecli.ExecDockerComposeStreamOptions
	execFn := func(ctx context.Context, opts composecli.ExecDockerComposeStreamOptions) *composecli.ExecDockerComposeStreamResult {
		mu.Lock()
		calls = append(calls, composecli.ExecDockerComposeStreamOptions{
			ExecPath:      opts.ExecPath,
			Args:          append([]string(nil), opts.Args...),
			OperationName: opts.OperationName,
		})
		mu.Unlock()

		exitCh := make(chan int, 1)
		exitCh <- 0
		close(exitCh)
		return &composecli.ExecDockerComposeStreamResult{
			Reader:   io.NopCloser(strings.NewReader("ok\r\n")),
			ExitCode: exitCh,
		}
	}
	return execFn, &calls
}

func TestComposeCreateUpSourceUsesSpecifiedComposeFileAndStopsOnSaveError(t *testing.T) {
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

	resolvedProjADir, err := filepath.EvalSymlinks(projADir)
	if err != nil {
		t.Fatal(err)
	}

	composeYamlPath := filepath.Join(resolvedProjADir, "compose.yaml")
	dockerComposeYmlPath := filepath.Join(resolvedProjADir, "docker-compose.yml")
	origComposeYaml := "services:\n  web:\n    image: nginx:1.25\n"
	origDockerComposeYml := "services:\n  sidecar:\n    image: alpine:3.19\n"
	if err := os.WriteFile(composeYamlPath, []byte(origComposeYaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerComposeYmlPath, []byte(origDockerComposeYml), 0644); err != nil {
		t.Fatal(err)
	}

	projBFile := filepath.Join(projBDir, "compose.yaml")
	origProjB := "services:\n  other:\n    image: busybox:1.36\n"
	if err := os.WriteFile(projBFile, []byte(origProjB), 0644); err != nil {
		t.Fatal(err)
	}

	cli := &composecli.Client{}

	t.Run("redeploy edited compose.yaml in multi-file directory", func(t *testing.T) {
		execFn, calls := okStreamExecutor()
		newYaml := "services:\n  web:\n    image: nginx:1.27\n"
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "proj-a",
			ComposeFile:    composeYamlPath,
			YamlContent:    newYaml,
			Force:          true,
			ComposeDir:     resolvedProjADir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"success"`) {
			t.Fatalf("expected success status in output, got: %s", output)
		}

		gotCompose, _ := os.ReadFile(composeYamlPath)
		if string(gotCompose) != newYaml {
			t.Fatalf("compose.yaml not updated: got=%q want=%q", string(gotCompose), newYaml)
		}
		gotDockerYml, _ := os.ReadFile(dockerComposeYmlPath)
		if string(gotDockerYml) != origDockerComposeYml {
			t.Fatalf("docker-compose.yml bytes changed unexpectedly: got=%q want=%q", string(gotDockerYml), origDockerComposeYml)
		}

		if len(*calls) != 2 {
			t.Fatalf("expected 2 executor calls (up + ps), got %d: %#v", len(*calls), *calls)
		}
		upCall := (*calls)[0]
		wantUpArgs := []string{"-f", composeYamlPath, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
		if upCall.ExecPath != resolvedProjADir || !reflect.DeepEqual(upCall.Args, wantUpArgs) {
			t.Fatalf("unexpected up call: dir=%q args=%#v (want dir=%q args=%#v)", upCall.ExecPath, upCall.Args, resolvedProjADir, wantUpArgs)
		}
		psCall := (*calls)[1]
		wantPsArgs := []string{"-f", composeYamlPath, "ps"}
		if psCall.ExecPath != resolvedProjADir || !reflect.DeepEqual(psCall.Args, wantPsArgs) {
			t.Fatalf("unexpected ps call: dir=%q args=%#v", psCall.ExecPath, psCall.Args)
		}
	})

	t.Run("legacy omitted composeFile on multi-file directory fails without touching files or invoking executor", func(t *testing.T) {
		beforeCompose, _ := os.ReadFile(composeYamlPath)
		beforeDockerYml, _ := os.ReadFile(dockerComposeYmlPath)

		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "proj-a",
			ComposeFile:    "",
			YamlContent:    "services:\n  web:\n    image: nginx:bad\n",
			Force:          true,
			ComposeDir:     resolvedProjADir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"error"`) {
			t.Fatalf("expected error status in output, got: %s", output)
		}
		if len(*calls) != 0 {
			t.Fatalf("expected 0 executor calls when save fails, got %#v", *calls)
		}
		if afterCompose, _ := os.ReadFile(composeYamlPath); string(afterCompose) != string(beforeCompose) {
			t.Fatalf("compose.yaml modified on failure")
		}
		if afterDockerYml, _ := os.ReadFile(dockerComposeYmlPath); string(afterDockerYml) != string(beforeDockerYml) {
			t.Fatalf("docker-compose.yml modified on failure")
		}
	})

	t.Run("cross-project composeFile fails without modifying files or invoking executor", func(t *testing.T) {
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "proj-a",
			ComposeFile:    projBFile,
			YamlContent:    "services:\n  hijack:\n    image: alpine:latest\n",
			Force:          true,
			ComposeDir:     resolvedProjADir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"error"`) {
			t.Fatalf("expected error status in output, got: %s", output)
		}
		if len(*calls) != 0 {
			t.Fatalf("expected 0 executor calls for cross-project file, got %#v", *calls)
		}
		if afterB, _ := os.ReadFile(projBFile); string(afterB) != origProjB {
			t.Fatalf("proj-b/compose.yaml modified on failure")
		}
	})

	t.Run("new project without composeFile creates default docker-compose.yaml and passes -f", func(t *testing.T) {
		resolvedAppDir, _ := filepath.EvalSymlinks(appDir)
		newDir := filepath.Join(resolvedAppDir, "fresh-proj")
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "fresh-proj",
			ComposeFile:    "",
			YamlContent:    "services:\n  app:\n    image: alpine:3.20\n",
			Force:          true,
			ComposeDir:     newDir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"success"`) {
			t.Fatalf("expected success status for fresh project, got: %s", output)
		}
		wantFile := filepath.Join(newDir, "docker-compose.yaml")
		if _, err := os.Stat(wantFile); err != nil {
			t.Fatalf("expected %s created, err=%v", wantFile, err)
		}
		if len(*calls) != 2 || len((*calls)[0].Args) < 2 || (*calls)[0].Args[0] != "-f" || (*calls)[0].Args[1] != wantFile {
			t.Fatalf("expected -f %q passed to up, got %#v", wantFile, *calls)
		}
	})
}
