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

func extractComposeFileFlags(args []string) []string {
	var flags []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" {
			flags = append(flags, "-f", args[i+1])
			i++
		}
	}
	return flags
}

func runComposeConfigServices(t *testing.T, execDir string, upArgs []string) []string {
	t.Helper()
	fileArgs := extractComposeFileFlags(upArgs)
	cfgArgs := append(append([]string(nil), fileArgs...), "config", "--services")
	res := composecli.ExecuteDockerComposeCommand(context.Background(), composecli.ExecDockerComposeOptions{
		ExecPath:      execDir,
		Args:          cfgArgs,
		OperationName: "verify compose services",
		NeedOutput:    true,
	})
	if res.Error != nil {
		t.Fatalf("docker compose config --services failed (args=%#v): %v\noutput: %s", cfgArgs, res.Error, string(res.Output))
	}
	var services []string
	for _, line := range strings.Split(string(res.Output), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			services = append(services, trimmed)
		}
	}
	return services
}

func TestComposeCreateUpSourcePreservesOverrideFilesAndRuntimeConfigFiles(t *testing.T) {
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

	t.Run("directory-discovered compose.yaml + compose.override.yaml keeps both services and only edits compose.yaml", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "override-dir-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		baseFile := filepath.Join(projDir, "compose.yaml")
		overrideFile := filepath.Join(projDir, "compose.override.yaml")

		origBase := "services:\n  api:\n    image: nginx:1.25\n"
		updatedBase := "services:\n  api:\n    image: nginx:1.27\n"
		origOverride := "services:\n  worker:\n    image: alpine:3.20\n"
		if err := os.WriteFile(baseFile, []byte(origBase), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(overrideFile, []byte(origOverride), 0644); err != nil {
			t.Fatal(err)
		}

		cli := &composecli.Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
				return nil, nil
			},
		}
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "override-dir-proj",
			ComposeFile:    baseFile,
			YamlContent:    updatedBase,
			Force:          true,
			ComposeDir:     projDir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"success"`) {
			t.Fatalf("expected success status, got: %s", output)
		}

		if gotBase, _ := os.ReadFile(baseFile); string(gotBase) != updatedBase {
			t.Fatalf("compose.yaml not updated: got=%q want=%q", string(gotBase), updatedBase)
		}
		if gotOverride, _ := os.ReadFile(overrideFile); string(gotOverride) != origOverride {
			t.Fatalf("compose.override.yaml must stay untouched: got=%q want=%q", string(gotOverride), origOverride)
		}

		if len(*calls) != 2 {
			t.Fatalf("expected up + ps calls, got %d: %#v", len(*calls), *calls)
		}
		wantUpArgs := []string{"-f", baseFile, "-f", overrideFile, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
		wantPsArgs := []string{"-f", baseFile, "-f", overrideFile, "ps"}
		if (*calls)[0].ExecPath != projDir || !reflect.DeepEqual((*calls)[0].Args, wantUpArgs) {
			t.Fatalf("unexpected up call: %#v (want dir=%q args=%#v)", (*calls)[0], projDir, wantUpArgs)
		}
		if (*calls)[1].ExecPath != projDir || !reflect.DeepEqual((*calls)[1].Args, wantPsArgs) {
			t.Fatalf("unexpected ps call: %#v (want dir=%q args=%#v)", (*calls)[1], projDir, wantPsArgs)
		}

		services := runComposeConfigServices(t, (*calls)[0].ExecPath, (*calls)[0].Args)
		wantServices := map[string]bool{"api": false, "worker": false}
		for _, svc := range services {
			wantServices[svc] = true
		}
		if !wantServices["api"] || !wantServices["worker"] || len(services) != 2 {
			t.Fatalf("expected services [api worker] with selected -f flags, got %#v", services)
		}
	})

	t.Run("explicit single-file runtime project ignores sibling compose.override.yaml", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "single-runtime-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		baseFile := filepath.Join(projDir, "compose.yaml")
		overrideFile := filepath.Join(projDir, "compose.override.yaml")

		origBase := "services:\n  api:\n    image: nginx:1.25\n"
		updatedBase := "services:\n  api:\n    image: nginx:1.27\n"
		origOverride := "services:\n  worker:\n    image: alpine:3.20\n"
		if err := os.WriteFile(baseFile, []byte(origBase), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(overrideFile, []byte(origOverride), 0644); err != nil {
			t.Fatal(err)
		}

		cli := &composecli.Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
				return []composecli.ListOutput{{
					Name:        "single-runtime-proj",
					Status:      "running(1)",
					ConfigFiles: baseFile,
				}}, nil
			},
		}
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "single-runtime-proj",
			ComposeFile:    baseFile,
			YamlContent:    updatedBase,
			Force:          true,
			ComposeDir:     projDir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"success"`) {
			t.Fatalf("expected success status, got: %s", output)
		}
		if len(*calls) != 2 {
			t.Fatalf("expected up + ps calls, got %d", len(*calls))
		}
		wantUpArgs := []string{"-f", baseFile, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
		wantPsArgs := []string{"-f", baseFile, "ps"}
		if !reflect.DeepEqual((*calls)[0].Args, wantUpArgs) || !reflect.DeepEqual((*calls)[1].Args, wantPsArgs) {
			t.Fatalf("expected single-file -f %q args, got up=%#v ps=%#v", baseFile, (*calls)[0].Args, (*calls)[1].Args)
		}

		services := runComposeConfigServices(t, (*calls)[0].ExecPath, (*calls)[0].Args)
		if !reflect.DeepEqual(services, []string{"api"}) {
			t.Fatalf("expected only [api] loaded when runtime ConfigFiles selects single file, got %#v", services)
		}
	})

	t.Run("runtime ConfigFiles conflict with edited target errors before writing or invoking executor", func(t *testing.T) {
		projDir := filepath.Join(resolvedAppDir, "conflict-runtime-proj")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatal(err)
		}
		composeYaml := filepath.Join(projDir, "compose.yaml")
		dockerComposeYml := filepath.Join(projDir, "docker-compose.yml")
		origCompose := "services:\n  api:\n    image: nginx:1.25\n"
		origLegacy := "services:\n  legacy:\n    image: busybox:1.36\n"
		if err := os.WriteFile(composeYaml, []byte(origCompose), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dockerComposeYml, []byte(origLegacy), 0644); err != nil {
			t.Fatal(err)
		}

		cli := &composecli.Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
				return []composecli.ListOutput{{
					Name:        "conflict-runtime-proj",
					Status:      "running(1)",
					ConfigFiles: dockerComposeYml,
				}}, nil
			},
		}
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "conflict-runtime-proj",
			ComposeFile:    composeYaml,
			YamlContent:    "services:\n  api:\n    image: nginx:1.28\n",
			Force:          true,
			ComposeDir:     projDir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"error"`) || !strings.Contains(output, composeYaml) || !strings.Contains(output, dockerComposeYml) {
			t.Fatalf("expected error naming conflicting paths %q and %q, got: %s", composeYaml, dockerComposeYml, output)
		}
		if len(*calls) != 0 {
			t.Fatalf("expected 0 executor calls on config file conflict, got %#v", *calls)
		}
		if got, _ := os.ReadFile(composeYaml); string(got) != origCompose {
			t.Fatalf("compose.yaml modified on conflict: %q", string(got))
		}
		if got, _ := os.ReadFile(dockerComposeYml); string(got) != origLegacy {
			t.Fatalf("docker-compose.yml modified on conflict: %q", string(got))
		}
	})

	t.Run("runtime ConfigFiles pointing to foreign project directory errors before writing or invoking executor", func(t *testing.T) {
		projADir := filepath.Join(resolvedAppDir, "foreign-check-a")
		projBDir := filepath.Join(resolvedAppDir, "foreign-check-b")
		for _, d := range []string{projADir, projBDir} {
			if err := os.MkdirAll(d, 0755); err != nil {
				t.Fatal(err)
			}
		}
		fileA := filepath.Join(projADir, "compose.yaml")
		fileB := filepath.Join(projBDir, "compose.yaml")
		origA := "services:\n  a:\n    image: alpine:3.19\n"
		origB := "services:\n  b:\n    image: alpine:3.20\n"
		if err := os.WriteFile(fileA, []byte(origA), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fileB, []byte(origB), 0644); err != nil {
			t.Fatal(err)
		}

		cli := &composecli.Client{
			RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
				return []composecli.ListOutput{{
					Name:        "foreign-check-a",
					Status:      "running(1)",
					ConfigFiles: fileB,
				}}, nil
			},
		}
		execFn, calls := okStreamExecutor()
		src := NewComposeCreateUpSource(ComposeCreateUpSourceOptions{
			ProjectName:    "foreign-check-a",
			ComposeFile:    fileA,
			YamlContent:    "services:\n  a:\n    image: alpine:3.21\n",
			Force:          true,
			ComposeDir:     projADir,
			ComposeClient:  cli,
			StreamExecutor: execFn,
		})

		output := drainCreateUpStream(t, src)
		if !strings.Contains(output, `"status":"error"`) || !strings.Contains(output, fileB) {
			t.Fatalf("expected error naming foreign path %q, got: %s", fileB, output)
		}
		if len(*calls) != 0 {
			t.Fatalf("expected 0 executor calls on ownership mismatch, got %#v", *calls)
		}
		if got, _ := os.ReadFile(fileA); string(got) != origA {
			t.Fatalf("fileA modified on ownership mismatch")
		}
		if got, _ := os.ReadFile(fileB); string(got) != origB {
			t.Fatalf("fileB modified on ownership mismatch")
		}
	})
}
