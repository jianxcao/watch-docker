package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
	"go.uber.org/zap"
)

type recordedDockerCall struct {
	pwd  string
	args []string
}

func installFakeDockerForCardTest(t *testing.T, logPath string) {
	t.Helper()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
line="PWD=$(pwd)"
for a in "$@"; do
  line="${line}	${a}"
done
printf '%s\n' "$line" >> "` + logPath + `"
echo "mock-docker: $*"
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readRecordedDockerCalls(t *testing.T, logPath string) []recordedDockerCall {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var calls []recordedDockerCall
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "\t")
		if len(parts) < 1 || !strings.HasPrefix(parts[0], "PWD=") {
			t.Fatalf("unexpected docker log line: %q", line)
		}
		calls = append(calls, recordedDockerCall{
			pwd:  strings.TrimPrefix(parts[0], "PWD="),
			args: parts[1:],
		})
	}
	return calls
}

func TestManagedComposeCardWebSockets(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
	prodFile := filepath.Join(siteDir, "prod.yaml")
	overrideFile := filepath.Join(siteDir, "override.yaml")
	for _, f := range []string{prodFile, overrideFile} {
		if err := os.WriteFile(f, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runtimeLister := func(ctx context.Context) ([]composecli.ListOutput, error) {
		return []composecli.ListOutput{{
			Name:        "blue",
			Status:      "running(1)",
			ConfigFiles: prodFile + "," + overrideFile,
		}}, nil
	}

	cli := &composecli.Client{RuntimeProjectsLister: runtimeLister}
	mgr := wsstream.NewStreamManager[[]byte]()
	t.Cleanup(mgr.Close)

	srv := &Server{
		logger:             zap.NewNop(),
		composeClient:      cli,
		streamManagerBytes: mgr,
	}
	router := gin.New()
	router.GET("/api/v1/compose/up/:projectName/ws", srv.handleComposeUpExistingWebSocket())
	router.GET("/api/v1/compose/pull/:projectName/ws", srv.handleComposePullWebSocket())

	httpSrv := httptest.NewServer(router)
	defer httpSrv.Close()
	wsBase := "ws" + strings.TrimPrefix(httpSrv.URL, "http")

	wantPrefix := []string{"-p", "blue", "-f", prodFile, "-f", overrideFile}
	wantUpArgs := append(append([]string{"compose"}, wantPrefix...), "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate")
	wantPsArgs := append(append([]string{"compose"}, wantPrefix...), "ps")
	wantPullArgs := append(append([]string{"compose"}, wantPrefix...), "--ansi", "always", "pull")

	inputs := []struct {
		name        string
		projectName string
		composeFile string
	}{
		{name: "single custom file via runtime name", projectName: "blue", composeFile: prodFile},
		{name: "comma-joined ConfigFiles via directory name", projectName: "site", composeFile: prodFile + "," + overrideFile},
	}

	for _, tc := range inputs {
		t.Run("up and ps/"+tc.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "docker-up.log")
			installFakeDockerForCardTest(t, logPath)

			u := wsBase + "/api/v1/compose/up/" + url.PathEscape(tc.projectName) + "/ws?composeFile=" + url.QueryEscape(tc.composeFile)
			conn, _, derr := websocket.DefaultDialer.Dial(u, nil)
			if derr != nil {
				t.Fatalf("dial up ws failed: %v", derr)
			}
			defer conn.Close()

			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			var frames []string
			for {
				_, msg, rerr := conn.ReadMessage()
				if len(msg) > 0 {
					frames = append(frames, string(msg))
				}
				if rerr != nil {
					break
				}
			}

			calls := readRecordedDockerCalls(t, logPath)
			if len(calls) != 2 {
				t.Fatalf("expected 2 docker calls (up + ps), got %d: %#v (frames=%q)", len(calls), calls, strings.Join(frames, ""))
			}
			if calls[0].pwd != siteDir || !slices.Equal(calls[0].args, wantUpArgs) {
				t.Fatalf("up call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", calls[0].pwd, calls[0].args, siteDir, wantUpArgs)
			}
			if calls[1].pwd != siteDir || !slices.Equal(calls[1].args, wantPsArgs) {
				t.Fatalf("ps call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", calls[1].pwd, calls[1].args, siteDir, wantPsArgs)
			}
		})

		t.Run("pull/"+tc.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "docker-pull.log")
			installFakeDockerForCardTest(t, logPath)

			u := wsBase + "/api/v1/compose/pull/" + url.PathEscape(tc.projectName) + "/ws?composeFile=" + url.QueryEscape(tc.composeFile)
			conn, _, derr := websocket.DefaultDialer.Dial(u, nil)
			if derr != nil {
				t.Fatalf("dial pull ws failed: %v", derr)
			}

			deadline := time.Now().Add(5 * time.Second)
			_ = conn.SetReadDeadline(deadline)
			var frames []string
			for {
				_, msg, rerr := conn.ReadMessage()
				if len(msg) > 0 {
					frames = append(frames, string(msg))
				}
				if calls := readRecordedDockerCalls(t, logPath); len(calls) > 0 {
					_ = conn.Close()
					break
				}
				if rerr != nil {
					_ = conn.Close()
					break
				}
			}

			calls := readRecordedDockerCalls(t, logPath)
			if len(calls) != 1 {
				t.Fatalf("expected 1 docker call (pull), got %d: %#v (frames=%q)", len(calls), calls, strings.Join(frames, ""))
			}
			if calls[0].pwd != siteDir || !slices.Equal(calls[0].args, wantPullArgs) {
				t.Fatalf("pull call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", calls[0].pwd, calls[0].args, siteDir, wantPullArgs)
			}
		})
	}

	t.Run("failed card requests execute zero docker commands", func(t *testing.T) {
		extFile := filepath.Join(tmpRoot, "external", "compose.yaml")
		if err := os.MkdirAll(filepath.Dir(extFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(extFile, []byte("services:\n  ext:\n    image: alpine:3.20\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		failLog := filepath.Join(t.TempDir(), "docker-fail.log")
		installFakeDockerForCardTest(t, failLog)

		failCases := []struct {
			name        string
			projectName string
			composeFile string
			lister      func(context.Context) ([]composecli.ListOutput, error)
		}{
			{
				name:        "out of order multi-file list",
				projectName: "blue",
				composeFile: overrideFile + "," + prodFile,
				lister:      runtimeLister,
			},
			{
				name:        "external unmanaged file",
				projectName: "blue",
				composeFile: extFile,
				lister:      runtimeLister,
			},
			{
				name:        "runtime lister failure",
				projectName: "blue",
				composeFile: prodFile,
				lister: func(ctx context.Context) ([]composecli.ListOutput, error) {
					return nil, errors.New("daemon offline")
				},
			},
		}

		for _, fc := range failCases {
			cli.RuntimeProjectsLister = fc.lister
			for _, op := range []string{"up", "pull"} {
				u := wsBase + "/api/v1/compose/" + op + "/" + url.PathEscape(fc.projectName) + "/ws?composeFile=" + url.QueryEscape(fc.composeFile)
				conn, _, derr := websocket.DefaultDialer.Dial(u, nil)
				if derr != nil {
					t.Fatalf("%s (%s) dial failed: %v", fc.name, op, derr)
				}
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				_, msg, _ := conn.ReadMessage()
				_ = conn.Close()
				if len(msg) == 0 {
					t.Fatalf("%s (%s) expected error frame on ws close", fc.name, op)
				}
			}
		}

		if calls := readRecordedDockerCalls(t, failLog); len(calls) != 0 {
			t.Fatalf("expected 0 docker calls on failed card requests, got %#v", calls)
		}
	})
}
