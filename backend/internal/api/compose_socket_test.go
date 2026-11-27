package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	for _, k := range []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR", "COMPOSE_DISABLE_ENV_FILE", "COMPOSE_ENV_FILES"} {
		key := k
		if orig, had := os.LookupEnv(key); had {
			t.Cleanup(func() { _ = os.Setenv(key, orig) })
			_ = os.Unsetenv(key)
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(key) })
		}
	}

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

	t.Run("draft card uses dotenv COMPOSE_FILE list", func(t *testing.T) {
		draftDir := filepath.Join(resolvedAppDir, "draft-site")
		if err := os.MkdirAll(draftDir, 0o755); err != nil {
			t.Fatal(err)
		}
		draftCompose := filepath.Join(draftDir, "compose.yaml")
		draftProd := filepath.Join(draftDir, "prod.yaml")
		draftOv := filepath.Join(draftDir, "site-override.yaml")
		origDraftCompose := "services:\n  web:\n    image: nginx:1.25\n"
		if err := os.WriteFile(draftCompose, []byte(origDraftCompose), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(draftProd, []byte("services:\n  production:\n    image: nginx:1.26\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(draftOv, []byte("services:\n  production:\n    restart: always\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(draftDir, ".env"), []byte("COMPOSE_FILE=prod.yaml:site-override.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		t.Setenv("COMPOSE_PATH_SEPARATOR", ":")
		prevLister := cli.RuntimeProjectsLister
		cli.RuntimeProjectsLister = func(ctx context.Context) ([]composecli.ListOutput, error) {
			return nil, nil
		}
		defer func() { cli.RuntimeProjectsLister = prevLister }()

		cards, lerr := cli.ListProjects(context.Background())
		if lerr != nil {
			t.Fatalf("ListProjects failed: %v", lerr)
		}
		var targetCard composecli.ComposeProject
		foundCard := false
		for _, card := range cards {
			if card.Name == "draft-site" {
				targetCard = card
				foundCard = true
				break
			}
		}
		if !foundCard || targetCard.ComposeFile != draftCompose {
			t.Fatalf("expected draft-site card pointing to %q, got %#v (cards=%#v)", draftCompose, targetCard, cards)
		}

		draftPrefix := []string{"-f", draftProd, "-f", draftOv}
		wantDraftUpArgs := append(append([]string{"compose"}, draftPrefix...), "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate")
		wantDraftPsArgs := append(append([]string{"compose"}, draftPrefix...), "ps")
		wantDraftPullArgs := append(append([]string{"compose"}, draftPrefix...), "--ansi", "always", "pull")

		upLog := filepath.Join(t.TempDir(), "draft-up.log")
		installFakeDockerForCardTest(t, upLog)
		uUp := wsBase + "/api/v1/compose/up/" + url.PathEscape(targetCard.Name) + "/ws?composeFile=" + url.QueryEscape(targetCard.ComposeFile)
		upConn, _, derr := websocket.DefaultDialer.Dial(uUp, nil)
		if derr != nil {
			t.Fatalf("dial draft up ws failed: %v", derr)
		}
		_ = upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var upFrames []string
		for {
			_, msg, rerr := upConn.ReadMessage()
			if len(msg) > 0 {
				upFrames = append(upFrames, string(msg))
			}
			if rerr != nil {
				break
			}
		}
		_ = upConn.Close()

		upCalls := readRecordedDockerCalls(t, upLog)
		if len(upCalls) != 2 {
			t.Fatalf("expected 2 docker calls (up + ps) for draft card, got %d: %#v (frames=%q)", len(upCalls), upCalls, strings.Join(upFrames, ""))
		}
		if upCalls[0].pwd != draftDir || !slices.Equal(upCalls[0].args, wantDraftUpArgs) {
			t.Fatalf("draft up call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", upCalls[0].pwd, upCalls[0].args, draftDir, wantDraftUpArgs)
		}
		if upCalls[1].pwd != draftDir || !slices.Equal(upCalls[1].args, wantDraftPsArgs) {
			t.Fatalf("draft ps call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", upCalls[1].pwd, upCalls[1].args, draftDir, wantDraftPsArgs)
		}

		pullLog := filepath.Join(t.TempDir(), "draft-pull.log")
		installFakeDockerForCardTest(t, pullLog)
		uPull := wsBase + "/api/v1/compose/pull/" + url.PathEscape(targetCard.Name) + "/ws?composeFile=" + url.QueryEscape(targetCard.ComposeFile)
		pullConn, _, derr := websocket.DefaultDialer.Dial(uPull, nil)
		if derr != nil {
			t.Fatalf("dial draft pull ws failed: %v", derr)
		}
		_ = pullConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var pullFrames []string
		for {
			_, msg, rerr := pullConn.ReadMessage()
			if len(msg) > 0 {
				pullFrames = append(pullFrames, string(msg))
			}
			if calls := readRecordedDockerCalls(t, pullLog); len(calls) > 0 {
				_ = pullConn.Close()
				break
			}
			if rerr != nil {
				_ = pullConn.Close()
				break
			}
		}
		pullCalls := readRecordedDockerCalls(t, pullLog)
		if len(pullCalls) != 1 || pullCalls[0].pwd != draftDir || !slices.Equal(pullCalls[0].args, wantDraftPullArgs) {
			t.Fatalf("draft pull call mismatch: calls=%#v want=(pwd=%q args=%#v) frames=%q", pullCalls, draftDir, wantDraftPullArgs, strings.Join(pullFrames, ""))
		}

		// 显式编辑未列入 COMPOSE_FILE 的 compose.yaml 仍报成员冲突且不改写磁盘原文件
		if _, serr := cli.SaveProjectPlanForDeploy(context.Background(), "draft-site", draftCompose, "services:\n  tampered:\n    image: busybox:1.36\n", true); serr == nil {
			t.Fatal("expected SaveProjectPlanForDeploy conflict when explicitly editing compose.yaml absent from COMPOSE_FILE")
		}
		if gotBytes, rerr := os.ReadFile(draftCompose); rerr != nil || string(gotBytes) != origDraftCompose {
			t.Fatalf("compose.yaml modified despite membership conflict: err=%v content=%q", rerr, string(gotBytes))
		}
	})

	t.Run("running custom COMPOSE_FILE card merged by name drives up, ps, and pull", func(t *testing.T) {
		subAppRaw := filepath.Join(t.TempDir(), "apps")
		if err := os.MkdirAll(subAppRaw, 0o755); err != nil {
			t.Fatal(err)
		}
		subAppResolved, err := filepath.EvalSymlinks(subAppRaw)
		if err != nil {
			t.Fatal(err)
		}

		subSiteDir := filepath.Join(subAppResolved, "site")
		if err := os.MkdirAll(subSiteDir, 0o755); err != nil {
			t.Fatal(err)
		}
		subCompose := filepath.Join(subSiteDir, "compose.yaml")
		subProd := filepath.Join(subSiteDir, "prod.yaml")
		subOv := filepath.Join(subSiteDir, "override.yaml")
		origSubCompose := "services:\n  web:\n    image: nginx:1.25\n"
		if err := os.WriteFile(subCompose, []byte(origSubCompose), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(subProd, []byte("services:\n  production:\n    image: nginx:1.26\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(subOv, []byte("services:\n  production:\n    restart: always\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		savedAppPath := conf.EnvCfg.APP_PATH
		conf.EnvCfg.APP_PATH = subAppResolved
		defer func() { conf.EnvCfg.APP_PATH = savedAppPath }()

		prevLister := cli.RuntimeProjectsLister
		cli.RuntimeProjectsLister = func(ctx context.Context) ([]composecli.ListOutput, error) {
			return []composecli.ListOutput{{
				Name:        "site",
				Status:      "running(2)",
				ConfigFiles: subProd + "," + subOv,
			}}, nil
		}
		defer func() { cli.RuntimeProjectsLister = prevLister }()

		cards, lerr := cli.ListProjects(context.Background())
		if lerr != nil || len(cards) != 1 {
			t.Fatalf("ListProjects() = %#v, %v", cards, lerr)
		}
		card := cards[0]
		if card.Name != "site" || card.Status != composecli.StatusRunning || card.ComposeFile != subCompose {
			t.Fatalf("expected merged running card pointing to %q, got %#v", subCompose, card)
		}

		mergedPrefix := []string{"-p", "site", "-f", subProd, "-f", subOv}
		wantMergedUpArgs := append(append([]string{"compose"}, mergedPrefix...), "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate")
		wantMergedPsArgs := append(append([]string{"compose"}, mergedPrefix...), "ps")
		wantMergedPullArgs := append(append([]string{"compose"}, mergedPrefix...), "--ansi", "always", "pull")

		upLog := filepath.Join(t.TempDir(), "merged-up.log")
		installFakeDockerForCardTest(t, upLog)
		uUp := wsBase + "/api/v1/compose/up/" + url.PathEscape(card.Name) + "/ws?composeFile=" + url.QueryEscape(card.ComposeFile)
		upConn, _, derr := websocket.DefaultDialer.Dial(uUp, nil)
		if derr != nil {
			t.Fatalf("dial merged up ws failed: %v", derr)
		}
		_ = upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var upFrames []string
		for {
			_, msg, rerr := upConn.ReadMessage()
			if len(msg) > 0 {
				upFrames = append(upFrames, string(msg))
			}
			if rerr != nil {
				break
			}
		}
		_ = upConn.Close()

		upCalls := readRecordedDockerCalls(t, upLog)
		if len(upCalls) != 2 {
			t.Fatalf("expected 2 docker calls (up + ps) for merged running card, got %d: %#v (frames=%q)", len(upCalls), upCalls, strings.Join(upFrames, ""))
		}
		if upCalls[0].pwd != subSiteDir || !slices.Equal(upCalls[0].args, wantMergedUpArgs) {
			t.Fatalf("merged up call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", upCalls[0].pwd, upCalls[0].args, subSiteDir, wantMergedUpArgs)
		}
		if upCalls[1].pwd != subSiteDir || !slices.Equal(upCalls[1].args, wantMergedPsArgs) {
			t.Fatalf("merged ps call mismatch: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", upCalls[1].pwd, upCalls[1].args, subSiteDir, wantMergedPsArgs)
		}

		pullLog := filepath.Join(t.TempDir(), "merged-pull.log")
		installFakeDockerForCardTest(t, pullLog)
		uPull := wsBase + "/api/v1/compose/pull/" + url.PathEscape(card.Name) + "/ws?composeFile=" + url.QueryEscape(card.ComposeFile)
		pullConn, _, derr := websocket.DefaultDialer.Dial(uPull, nil)
		if derr != nil {
			t.Fatalf("dial merged pull ws failed: %v", derr)
		}
		_ = pullConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var pullFrames []string
		for {
			_, msg, rerr := pullConn.ReadMessage()
			if len(msg) > 0 {
				pullFrames = append(pullFrames, string(msg))
			}
			if calls := readRecordedDockerCalls(t, pullLog); len(calls) > 0 {
				_ = pullConn.Close()
				break
			}
			if rerr != nil {
				_ = pullConn.Close()
				break
			}
		}
		pullCalls := readRecordedDockerCalls(t, pullLog)
		if len(pullCalls) != 1 || pullCalls[0].pwd != subSiteDir || !slices.Equal(pullCalls[0].args, wantMergedPullArgs) {
			t.Fatalf("merged pull call mismatch: calls=%#v want=(pwd=%q args=%#v) frames=%q", pullCalls, subSiteDir, wantMergedPullArgs, strings.Join(pullFrames, ""))
		}
	})

	t.Run("failed card requests execute zero docker commands", func(t *testing.T) {
		prevLister := cli.RuntimeProjectsLister
		defer func() { cli.RuntimeProjectsLister = prevLister }()
		extFile := filepath.Join(tmpRoot, "external", "compose.yaml")
		if err := os.MkdirAll(filepath.Dir(extFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(extFile, []byte("services:\n  ext:\n    image: alpine:3.20\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		emptyDraftDir := filepath.Join(resolvedAppDir, "empty-env-site")
		if err := os.MkdirAll(emptyDraftDir, 0o755); err != nil {
			t.Fatal(err)
		}
		emptyDraftCompose := filepath.Join(emptyDraftDir, "compose.yaml")
		if err := os.WriteFile(emptyDraftCompose, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(emptyDraftDir, ".env"), []byte("COMPOSE_FILE=\n"), 0o644); err != nil {
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
				name:        "empty COMPOSE_FILE in dotenv",
				projectName: "empty-env-site",
				composeFile: emptyDraftCompose,
				lister: func(ctx context.Context) ([]composecli.ListOutput, error) {
					return nil, nil
				},
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

func TestComposeCardWebSocketConcurrencyAndPlanIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, k := range []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR", "COMPOSE_DISABLE_ENV_FILE", "COMPOSE_ENV_FILES"} {
		key := k
		if orig, had := os.LookupEnv(key); had {
			t.Cleanup(func() { _ = os.Setenv(key, orig) })
			_ = os.Unsetenv(key)
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(key) })
		}
	}

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

	dualDir := filepath.Join(resolvedAppDir, "dual")
	if err := os.MkdirAll(dualDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeYaml := filepath.Join(dualDir, "compose.yaml")
	dockerComposeYaml := filepath.Join(dualDir, "docker-compose.yaml")
	if err := os.WriteFile(composeYaml, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerComposeYaml, []byte("services:\n  alt:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var runtimeMu sync.Mutex
	var runtimeLister func(context.Context) ([]composecli.ListOutput, error)
	setRuntimeLister := func(fn func(context.Context) ([]composecli.ListOutput, error)) {
		runtimeMu.Lock()
		runtimeLister = fn
		runtimeMu.Unlock()
	}
	setRuntimeProjects := func(items []composecli.ListOutput) {
		cloned := slices.Clone(items)
		setRuntimeLister(func(ctx context.Context) ([]composecli.ListOutput, error) {
			return slices.Clone(cloned), nil
		})
	}
	setRuntimeProjects(nil)

	cli := &composecli.Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
			runtimeMu.Lock()
			fn := runtimeLister
			runtimeMu.Unlock()
			if fn == nil {
				return nil, nil
			}
			return fn(ctx)
		},
	}
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
	router.GET("/api/v1/compose/logs/:projectName/ws", srv.handleComposeLogsWebSocket())
	router.GET("/api/v1/compose/create-and-up/ws", srv.handleComposeCreateAndUpWebSocket())

	httpSrv := httptest.NewServer(router)
	defer httpSrv.Close()
	wsBase := "ws" + strings.TrimPrefix(httpSrv.URL, "http")

	installGatedFakeDocker := func(t *testing.T, logPath, gateDir string) {
		t.Helper()
		binDir := t.TempDir()
		dockerPath := filepath.Join(binDir, "docker")
		script := `#!/bin/sh
line="PWD=$(pwd)"
subcmd=""
for a in "$@"; do
  line="${line}	${a}"
  case "$a" in
    ps|logs|up|pull) subcmd="$a" ;;
  esac
done
printf '%s\n' "$line" >> "` + logPath + `"
if [ "$subcmd" = "ps" ]; then
  echo "ps-done:$*"
  exit 0
fi
echo "started:$*"
gate=""
if [ "$subcmd" = "logs" ]; then
  gate="` + gateDir + `/block_logs"
else
  case "$*" in
    *docker-compose.yaml*) gate="` + gateDir + `/block_docker_compose" ;;
    *compose.yaml*) gate="` + gateDir + `/block_compose" ;;
  esac
fi
n=0
while [ -n "$gate" ] && [ -e "$gate" ] && [ "$n" -lt 250 ]; do
  sleep 0.02
  n=$((n + 1))
done
if [ "$subcmd" = "up" ] && [ -e "` + gateDir + `/fail_up" ]; then
  echo "mock-up-failed:$*" >&2
  exit 1
fi
echo "finished:$*"
`
		if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	type wsStatusFrame struct {
		msgType int
		status  string
		message string
	}
	type wsStreamRecorder struct {
		mu       sync.Mutex
		frames   []string
		msgTypes []int
		done     chan struct{}
		conn     *websocket.Conn
	}
	startRecorder := func(t *testing.T, conn *websocket.Conn) *wsStreamRecorder {
		t.Helper()
		rec := &wsStreamRecorder{done: make(chan struct{}), conn: conn}
		go func() {
			defer close(rec.done)
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
			for {
				mt, msg, rerr := conn.ReadMessage()
				if len(msg) > 0 {
					rec.mu.Lock()
					rec.frames = append(rec.frames, string(msg))
					rec.msgTypes = append(rec.msgTypes, mt)
					rec.mu.Unlock()
				}
				if rerr != nil {
					return
				}
			}
		}()
		return rec
	}
	startWS := func(t *testing.T, op, projectName, composeFile string) *wsStreamRecorder {
		t.Helper()
		u := wsBase + "/api/v1/compose/" + op + "/" + url.PathEscape(projectName) + "/ws?composeFile=" + url.QueryEscape(composeFile)
		conn, _, derr := websocket.DefaultDialer.Dial(u, nil)
		if derr != nil {
			t.Fatalf("dial %s ws failed: %v", op, derr)
		}
		return startRecorder(t, conn)
	}
	startCreateUpWS := func(t *testing.T, name, composeFile, yamlContent string, force bool) *wsStreamRecorder {
		t.Helper()
		u := wsBase + "/api/v1/compose/create-and-up/ws"
		conn, _, derr := websocket.DefaultDialer.Dial(u, nil)
		if derr != nil {
			t.Fatalf("dial create-and-up ws failed: %v", derr)
		}
		req := map[string]any{
			"name":        name,
			"composeFile": composeFile,
			"yamlContent": yamlContent,
			"force":       force,
		}
		if err := conn.WriteJSON(req); err != nil {
			_ = conn.Close()
			t.Fatalf("write create-and-up payload failed: %v", err)
		}
		return startRecorder(t, conn)
	}
	extractStatusFrames := func(rec *wsStreamRecorder) []wsStatusFrame {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		var out []wsStatusFrame
		for i, f := range rec.frames {
			const prefix = "\x00JSON:"
			idx := strings.Index(f, prefix)
			if idx < 0 {
				continue
			}
			var parsed struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(f[idx+len(prefix):]), &parsed); err != nil {
				continue
			}
			out = append(out, wsStatusFrame{
				msgType: rec.msgTypes[i],
				status:  parsed.Status,
				message: parsed.Message,
			})
		}
		return out
	}
	waitContains := func(t *testing.T, rec *wsStreamRecorder, substr string) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			rec.mu.Lock()
			joined := strings.Join(rec.frames, "")
			rec.mu.Unlock()
			if strings.Contains(joined, substr) {
				return
			}
			select {
			case <-rec.done:
				rec.mu.Lock()
				joined = strings.Join(rec.frames, "")
				rec.mu.Unlock()
				if strings.Contains(joined, substr) {
					return
				}
				t.Fatalf("stream ended before observing %q; frames=%q", substr, joined)
			case <-time.After(15 * time.Millisecond):
			}
		}
		rec.mu.Lock()
		joined := strings.Join(rec.frames, "")
		rec.mu.Unlock()
		t.Fatalf("timed out waiting for %q; frames=%q", substr, joined)
	}
	waitDone := func(t *testing.T, rec *wsStreamRecorder) string {
		t.Helper()
		select {
		case <-rec.done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for websocket stream to close")
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return strings.Join(rec.frames, "")
	}
	waitHubCountZero := func(t *testing.T) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if mgr.GetHubCount() == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("expected hub count 0 after streams exit, got %d", mgr.GetHubCount())
	}

	// 1. 同一项目、相同 compose.yaml 并发 pull：共享同一流，只执行一次 pull，并在结束后清理 Hub
	t.Run("concurrent pull same plan shares stream and cleans up hub", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "pull-same.log")
		gateDir := t.TempDir()
		gateFile := filepath.Join(gateDir, "block_compose")
		if err := os.WriteFile(gateFile, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(gateFile)
		installGatedFakeDocker(t, logPath, gateDir)

		rec1 := startWS(t, "pull", "dual", composeYaml)
		waitContains(t, rec1, "started:")

		rec2 := startWS(t, "pull", "dual", composeYaml)
		time.Sleep(60 * time.Millisecond)
		if got := mgr.GetHubCount(); got != 1 {
			t.Fatalf("expected 1 shared hub during identical pull, got %d", got)
		}
		if err := os.Remove(gateFile); err != nil {
			t.Fatal(err)
		}

		out1 := waitDone(t, rec1)
		out2 := waitDone(t, rec2)
		if !strings.Contains(out1, "finished:") || !strings.Contains(out2, "finished:") {
			t.Fatalf("expected completion frames on both clients, out1=%q out2=%q", out1, out2)
		}
		calls := readRecordedDockerCalls(t, logPath)
		if len(calls) != 1 {
			t.Fatalf("expected 1 docker pull call for shared stream, got %d: %#v", len(calls), calls)
		}
		waitHubCountZero(t)
	})

	// 2. 同一项目名称下、不同文件列表并发 pull：必须隔离为两条流、分别执行各自命令、互不串音并在关闭后全部清理
	t.Run("concurrent pull different plans in same dir stay isolated", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "pull-diff.log")
		gateDir := t.TempDir()
		gate1 := filepath.Join(gateDir, "block_compose")
		gate2 := filepath.Join(gateDir, "block_docker_compose")
		for _, gf := range []string{gate1, gate2} {
			if err := os.WriteFile(gf, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			gf := gf
			defer os.Remove(gf)
		}
		installGatedFakeDocker(t, logPath, gateDir)

		rec1 := startWS(t, "pull", "dual", composeYaml)
		waitContains(t, rec1, composeYaml)

		rec2 := startWS(t, "pull", "dual", dockerComposeYaml)
		waitContains(t, rec2, dockerComposeYaml)
		if got := mgr.GetHubCount(); got != 2 {
			t.Fatalf("expected 2 isolated hubs while both pulls run, got %d", got)
		}

		_ = os.Remove(gate1)
		_ = os.Remove(gate2)
		out1 := waitDone(t, rec1)
		out2 := waitDone(t, rec2)
		if strings.Contains(out1, dockerComposeYaml) || !strings.Contains(out1, composeYaml) {
			t.Fatalf("stream 1 polluted or missing compose.yaml output: %q", out1)
		}
		if strings.Contains(out2, composeYaml) || !strings.Contains(out2, dockerComposeYaml) {
			t.Fatalf("stream 2 polluted or missing docker-compose.yaml output: %q", out2)
		}
		calls := readRecordedDockerCalls(t, logPath)
		if len(calls) != 2 {
			t.Fatalf("expected 2 independent pull calls, got %d: %#v", len(calls), calls)
		}
		waitHubCountZero(t)
	})

	// 3. 同一项目、相同 compose.yaml 并发 up：共享同一流，只执行一次 up + ps
	t.Run("concurrent up same plan shares stream and cleans up hub", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "up-same.log")
		gateDir := t.TempDir()
		gateFile := filepath.Join(gateDir, "block_compose")
		if err := os.WriteFile(gateFile, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(gateFile)
		installGatedFakeDocker(t, logPath, gateDir)

		rec1 := startWS(t, "up", "dual", composeYaml)
		waitContains(t, rec1, "started:")

		rec2 := startWS(t, "up", "dual", composeYaml)
		time.Sleep(60 * time.Millisecond)
		if got := mgr.GetHubCount(); got != 1 {
			t.Fatalf("expected 1 shared up hub, got %d", got)
		}
		if err := os.Remove(gateFile); err != nil {
			t.Fatal(err)
		}

		out1 := waitDone(t, rec1)
		out2 := waitDone(t, rec2)
		if !strings.Contains(out1, `"status":"success"`) || !strings.Contains(out2, `"status":"success"`) {
			t.Fatalf("expected success JSON on both shared up clients, out1=%q out2=%q", out1, out2)
		}
		calls := readRecordedDockerCalls(t, logPath)
		if len(calls) != 2 {
			t.Fatalf("expected 2 docker calls (1 up + 1 ps), got %d: %#v", len(calls), calls)
		}
		waitHubCountZero(t)
	})

	// 4. 同目录不同文件的并发 up 请求：第二个不同计划不能共享第一个成功结果或并发重建，必须收到明确冲突错误；待第一个结束后再发起第二个可以正常执行
	t.Run("concurrent up different plans rejects conflicting deployment without cross-plan success", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "up-diff.log")
		gateDir := t.TempDir()
		gate1 := filepath.Join(gateDir, "block_compose")
		if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(gate1)
		installGatedFakeDocker(t, logPath, gateDir)

		rec1 := startWS(t, "up", "dual", composeYaml)
		waitContains(t, rec1, composeYaml)

		rec2 := startWS(t, "up", "dual", dockerComposeYaml)
		out2 := waitDone(t, rec2)
		if strings.Contains(out2, `"status":"success"`) || !strings.Contains(out2, `"status":"error"`) {
			t.Fatalf("conflicting concurrent up must return explicit error and never report success, got=%q", out2)
		}

		_ = os.Remove(gate1)
		out1 := waitDone(t, rec1)
		if !strings.Contains(out1, `"status":"success"`) {
			t.Fatalf("expected first up to complete cleanly, got %q", out1)
		}
		callsDuringFirst := readRecordedDockerCalls(t, logPath)
		for _, c := range callsDuringFirst {
			if slices.Contains(c.args, dockerComposeYaml) {
				t.Fatalf("conflicting second plan executed before first completed: %#v", callsDuringFirst)
			}
		}
		waitHubCountZero(t)

		// 第一个部署完成并清理后，再针对 docker-compose.yaml 发起 up 必须正常执行
		rec3 := startWS(t, "up", "dual", dockerComposeYaml)
		out3 := waitDone(t, rec3)
		if !strings.Contains(out3, `"status":"success"`) {
			t.Fatalf("expected sequential second up to succeed, got %q", out3)
		}
		callsAfter := readRecordedDockerCalls(t, logPath)
		if len(callsAfter) != 4 {
			t.Fatalf("expected 4 total calls (up+ps for each completed plan), got %d: %#v", len(callsAfter), callsAfter)
		}
		waitHubCountZero(t)
	})

	// 5. Item 1: create-and-up/ws 并发冲突隔离、与卡片 up 对称互斥，以及失败/退出/断连后槽位释放
	t.Run("create-and-up and card up serialize per target with binary status frames and clean release", func(t *testing.T) {
		origCompose := "services:\n  web:\n    image: nginx:1.25\n"
		origAlt := "services:\n  alt:\n    image: alpine:3.20\n"
		resetDualFiles := func(t *testing.T) {
			t.Helper()
			if err := os.WriteFile(composeYaml, []byte(origCompose), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dockerComposeYaml, []byte(origAlt), 0o644); err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(filepath.Join(dualDir, ".env"))
			setRuntimeProjects(nil)
		}

		// 5a. 同目录两个 create-and-up/ws 请求写入不同文件：第二个直接返回二进制 JSON error，放行后可重试成功
		t.Run("two create-and-up requests on different files", func(t *testing.T) {
			resetDualFiles(t)
			logPath := filepath.Join(t.TempDir(), "editor-diff.log")
			gateDir := t.TempDir()
			gate1 := filepath.Join(gateDir, "block_compose")
			if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(gate1)
			installGatedFakeDocker(t, logPath, gateDir)

			yamlA := "services:\n  web:\n    image: nginx:1.26-a\n"
			yamlB := "services:\n  alt:\n    image: alpine:3.21-b\n"

			rec1 := startCreateUpWS(t, "dual", composeYaml, yamlA, true)
			waitContains(t, rec1, "started:")

			rec2 := startCreateUpWS(t, "dual", dockerComposeYaml, yamlB, true)
			out2 := waitDone(t, rec2)
			statuses2 := extractStatusFrames(rec2)
			if len(statuses2) != 1 || statuses2[0].msgType != websocket.BinaryMessage || statuses2[0].status != "error" || !strings.Contains(statuses2[0].message, "正在部署中") {
				t.Fatalf("expected single binary error conflict status frame on rec2, got statuses=%#v out=%q", statuses2, out2)
			}

			if got, _ := os.ReadFile(composeYaml); string(got) != yamlA {
				t.Fatalf("compose.yaml mismatch: got=%q want=%q", string(got), yamlA)
			}
			if got, _ := os.ReadFile(dockerComposeYaml); string(got) != origAlt {
				t.Fatalf("docker-compose.yaml was overwritten during conflicting request: got=%q want=%q", string(got), origAlt)
			}

			_ = os.Remove(gate1)
			out1 := waitDone(t, rec1)
			statuses1 := extractStatusFrames(rec1)
			if len(statuses1) != 1 || statuses1[0].status != "success" {
				t.Fatalf("expected rec1 success, got statuses=%#v out=%q", statuses1, out1)
			}
			waitHubCountZero(t)

			callsMid := readRecordedDockerCalls(t, logPath)
			if len(callsMid) != 2 {
				t.Fatalf("expected 2 calls (1 up + 1 ps) during first deploy, got %#v", callsMid)
			}

			// 放行后单独重试第二个请求，必须成功并写入 yamlB
			rec3 := startCreateUpWS(t, "dual", dockerComposeYaml, yamlB, true)
			out3 := waitDone(t, rec3)
			statuses3 := extractStatusFrames(rec3)
			if len(statuses3) != 1 || statuses3[0].status != "success" {
				t.Fatalf("expected retry rec3 success, got statuses=%#v out=%q", statuses3, out3)
			}
			if got, _ := os.ReadFile(dockerComposeYaml); string(got) != yamlB {
				t.Fatalf("docker-compose.yaml not updated to yamlB after retry: got=%q", string(got))
			}
			if got, _ := os.ReadFile(composeYaml); string(got) != yamlA {
				t.Fatalf("compose.yaml unexpectedly mutated: got=%q", string(got))
			}
			waitHubCountZero(t)
		})

		// 5b. 同一文件不同 YAML 内容并发保存
		t.Run("two create-and-up requests on same file with different yaml", func(t *testing.T) {
			resetDualFiles(t)
			logPath := filepath.Join(t.TempDir(), "editor-same.log")
			gateDir := t.TempDir()
			gate1 := filepath.Join(gateDir, "block_compose")
			if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(gate1)
			installGatedFakeDocker(t, logPath, gateDir)

			yamlC := "services:\n  web:\n    image: nginx:1.27-c\n"
			yamlD := "services:\n  web:\n    image: nginx:1.28-d\n"

			rec1 := startCreateUpWS(t, "dual", composeYaml, yamlC, true)
			waitContains(t, rec1, "started:")

			rec2 := startCreateUpWS(t, "dual", composeYaml, yamlD, true)
			out2 := waitDone(t, rec2)
			statuses2 := extractStatusFrames(rec2)
			if len(statuses2) != 1 || statuses2[0].msgType != websocket.BinaryMessage || statuses2[0].status != "error" {
				t.Fatalf("expected binary conflict error for same-file concurrent save, got %#v out=%q", statuses2, out2)
			}
			if got, _ := os.ReadFile(composeYaml); string(got) != yamlC {
				t.Fatalf("compose.yaml overwritten by conflicting second writer: got=%q want=%q", string(got), yamlC)
			}

			_ = os.Remove(gate1)
			out1 := waitDone(t, rec1)
			if statuses1 := extractStatusFrames(rec1); len(statuses1) != 1 || statuses1[0].status != "success" {
				t.Fatalf("expected rec1 success, got %#v out=%q", statuses1, out1)
			}
			waitHubCountZero(t)

			rec3 := startCreateUpWS(t, "dual", composeYaml, yamlD, true)
			_ = waitDone(t, rec3)
			if statuses3 := extractStatusFrames(rec3); len(statuses3) != 1 || statuses3[0].status != "success" {
				t.Fatalf("expected retry rec3 success, got %#v", statuses3)
			}
			if got, _ := os.ReadFile(composeYaml); string(got) != yamlD {
				t.Fatalf("expected compose.yaml updated to yamlD, got=%q", string(got))
			}
			waitHubCountZero(t)
		})

		// 5c. 卡片 up 先到，编辑器保存后到 -> 编辑器返回冲突且不改写磁盘
		t.Run("card up blocks concurrent editor save and releases afterward", func(t *testing.T) {
			resetDualFiles(t)
			logPath := filepath.Join(t.TempDir(), "card-then-editor.log")
			gateDir := t.TempDir()
			gate1 := filepath.Join(gateDir, "block_compose")
			if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(gate1)
			installGatedFakeDocker(t, logPath, gateDir)

			recCard := startWS(t, "up", "dual", composeYaml)
			waitContains(t, recCard, "started:")

			yamlEdit := "services:\n  alt:\n    image: alpine:3.21-edited\n"
			recEditor := startCreateUpWS(t, "dual", dockerComposeYaml, yamlEdit, true)
			_ = waitDone(t, recEditor)
			edStatuses := extractStatusFrames(recEditor)
			if len(edStatuses) != 1 || edStatuses[0].msgType != websocket.BinaryMessage || edStatuses[0].status != "error" {
				t.Fatalf("expected editor binary conflict error while card up is active, got %#v", edStatuses)
			}
			if got, _ := os.ReadFile(dockerComposeYaml); string(got) != origAlt {
				t.Fatalf("docker-compose.yaml mutated while card up active: %q", string(got))
			}

			_ = os.Remove(gate1)
			_ = waitDone(t, recCard)
			if cStatuses := extractStatusFrames(recCard); len(cStatuses) != 1 || cStatuses[0].status != "success" {
				t.Fatalf("expected card up success, got %#v", cStatuses)
			}
			waitHubCountZero(t)

			recEditorRetry := startCreateUpWS(t, "dual", dockerComposeYaml, yamlEdit, true)
			_ = waitDone(t, recEditorRetry)
			if rStatuses := extractStatusFrames(recEditorRetry); len(rStatuses) != 1 || rStatuses[0].status != "success" {
				t.Fatalf("expected editor retry success after card up finished, got %#v", rStatuses)
			}
			if got, _ := os.ReadFile(dockerComposeYaml); string(got) != yamlEdit {
				t.Fatalf("expected docker-compose.yaml written after retry, got %q", string(got))
			}
			waitHubCountZero(t)
		})

		// 5d. 编辑器保存先到，卡片 up 后到 -> 卡片返回冲突且不执行第二条 up
		t.Run("editor save blocks concurrent card up and releases afterward", func(t *testing.T) {
			resetDualFiles(t)
			logPath := filepath.Join(t.TempDir(), "editor-then-card.log")
			gateDir := t.TempDir()
			gate1 := filepath.Join(gateDir, "block_compose")
			if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(gate1)
			installGatedFakeDocker(t, logPath, gateDir)

			yamlEdit := "services:\n  web:\n    image: nginx:1.27-first\n"
			recEditor := startCreateUpWS(t, "dual", composeYaml, yamlEdit, true)
			waitContains(t, recEditor, "started:")

			for _, cardFile := range []string{composeYaml, dockerComposeYaml} {
				recCard := startWS(t, "up", "dual", cardFile)
				_ = waitDone(t, recCard)
				cStatuses := extractStatusFrames(recCard)
				if len(cStatuses) != 1 || cStatuses[0].msgType != websocket.BinaryMessage || cStatuses[0].status != "error" {
					t.Fatalf("expected card up conflict on %s while editor save active, got %#v", cardFile, cStatuses)
				}
			}

			_ = os.Remove(gate1)
			_ = waitDone(t, recEditor)
			if eStatuses := extractStatusFrames(recEditor); len(eStatuses) != 1 || eStatuses[0].status != "success" {
				t.Fatalf("expected editor deploy success, got %#v", eStatuses)
			}
			callsMid := readRecordedDockerCalls(t, logPath)
			if len(callsMid) != 2 {
				t.Fatalf("expected only editor up+ps calls before release, got %#v", callsMid)
			}
			waitHubCountZero(t)

			recCardAfter := startWS(t, "up", "dual", composeYaml)
			_ = waitDone(t, recCardAfter)
			if aStatuses := extractStatusFrames(recCardAfter); len(aStatuses) != 1 || aStatuses[0].status != "success" {
				t.Fatalf("expected card up success after editor deploy released, got %#v", aStatuses)
			}
			waitHubCountZero(t)
		})

		// 5e. 启动失败、命令非零退出及最后订阅者提前断开后，部署槽位必须正确释放且可重试
		t.Run("slot releases on save error, startup error, exit 1, and last subscriber disconnect", func(t *testing.T) {
			resetDualFiles(t)

			// 1) SaveProjectPlanForDeploy 错误（.env 排除 target）
			envPath := filepath.Join(dualDir, ".env")
			if err := os.WriteFile(envPath, []byte("COMPOSE_FILE=compose.yaml\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			recSaveErr := startCreateUpWS(t, "dual", dockerComposeYaml, "services:\n  alt:\n    image: alpine:3.21\n", true)
			_ = waitDone(t, recSaveErr)
			if sErr := extractStatusFrames(recSaveErr); len(sErr) != 1 || sErr[0].status != "error" {
				t.Fatalf("expected error status on SaveProjectPlanForDeploy failure, got %#v", sErr)
			}
			_ = os.Remove(envPath)
			waitHubCountZero(t)

			// 2) docker 可执行文件缺失（Start 失败）
			t.Setenv("PATH", t.TempDir())
			recStartErr := startCreateUpWS(t, "dual", dockerComposeYaml, "services:\n  alt:\n    image: alpine:3.21\n", true)
			_ = waitDone(t, recStartErr)
			if stErr := extractStatusFrames(recStartErr); len(stErr) != 1 || stErr[0].status != "error" {
				t.Fatalf("expected error status when docker binary missing, got %#v", stErr)
			}
			waitHubCountZero(t)

			// 3) docker compose up 非零退出
			logPath := filepath.Join(t.TempDir(), "release.log")
			gateDir := t.TempDir()
			failUpFile := filepath.Join(gateDir, "fail_up")
			if err := os.WriteFile(failUpFile, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			installGatedFakeDocker(t, logPath, gateDir)

			recExitErr := startCreateUpWS(t, "dual", dockerComposeYaml, "services:\n  alt:\n    image: alpine:3.21\n", true)
			_ = waitDone(t, recExitErr)
			if exErr := extractStatusFrames(recExitErr); len(exErr) != 1 || exErr[0].status != "error" {
				t.Fatalf("expected error status when compose up exits 1, got %#v", exErr)
			}
			_ = os.Remove(failUpFile)
			waitHubCountZero(t)

			// 4) 阻塞执行过程中最后一个客户端提前断开连接：中止命令并释放登记，之后可重试成功
			gate1 := filepath.Join(gateDir, "block_compose")
			if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(gate1)

			recDisconnect := startCreateUpWS(t, "dual", composeYaml, "services:\n  web:\n    image: nginx:1.27-abort\n", true)
			waitContains(t, recDisconnect, "started:")
			_ = recDisconnect.conn.Close()
			_ = waitDone(t, recDisconnect)
			waitHubCountZero(t)

			// 等待子进程退出并释放槽位后立即针对 docker-compose.yaml 重试应成功
			var retryOK bool
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				recRetry := startWS(t, "up", "dual", dockerComposeYaml)
				_ = waitDone(t, recRetry)
				if rs := extractStatusFrames(recRetry); len(rs) == 1 && rs[0].status == "success" {
					retryOK = true
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if !retryOK {
				t.Fatalf("expected slot release and successful retry after last subscriber disconnected")
			}
			waitHubCountZero(t)
		})
	})

	// 6. Item 2: 首次部署跨越运行时发现（顶层 name 或 .env COMPOSE_PROJECT_NAME）保持互斥且不强行注入目录 -p
	t.Run("first deploy across runtime discovery stays exclusive without injecting directory -p", func(t *testing.T) {
		cases := []struct {
			name        string
			dirName     string
			composeBody string
			envBody     string
		}{
			{
				name:        "top-level compose name: blue",
				dirName:     "named-stack",
				composeBody: "name: blue\nservices:\n  web:\n    image: nginx:1.25\n",
			},
			{
				name:        "dotenv COMPOSE_PROJECT_NAME=blue",
				dirName:     "env-named-stack",
				composeBody: "services:\n  web:\n    image: nginx:1.25\n",
				envBody:     "COMPOSE_PROJECT_NAME=blue\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				setRuntimeProjects(nil)
				projDir := filepath.Join(resolvedAppDir, tc.dirName)
				if err := os.MkdirAll(projDir, 0o755); err != nil {
					t.Fatal(err)
				}
				cFile := filepath.Join(projDir, "compose.yaml")
				altFile := filepath.Join(projDir, "docker-compose.yaml")
				if err := os.WriteFile(cFile, []byte(tc.composeBody), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(altFile, []byte("services:\n  other:\n    image: alpine:3.20\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if tc.envBody != "" {
					if err := os.WriteFile(filepath.Join(projDir, ".env"), []byte(tc.envBody), 0o644); err != nil {
						t.Fatal(err)
					}
				}

				logPath := filepath.Join(t.TempDir(), "first-deploy.log")
				gateDir := t.TempDir()
				gate1 := filepath.Join(gateDir, "block_compose")
				if err := os.WriteFile(gate1, []byte("1"), 0o644); err != nil {
					t.Fatal(err)
				}
				defer os.Remove(gate1)
				installGatedFakeDocker(t, logPath, gateDir)

				// 首次部署前 compose ls 为空
				rec1 := startWS(t, "up", tc.dirName, cFile)
				waitContains(t, rec1, "started:")

				// 部署执行期间运行时列表开始返回 blue 与同一文件 cFile
				setRuntimeProjects([]composecli.ListOutput{{
					Name:        "blue",
					Status:      "running(1)",
					ConfigFiles: cFile,
				}})

				// 不同配置（docker-compose.yaml）在首deploy仍阻塞时必须被拒绝，不能触发第二条 up
				recConflict := startWS(t, "up", tc.dirName, altFile)
				outConflict := waitDone(t, recConflict)
				if !strings.Contains(outConflict, "冲突") || strings.Contains(outConflict, "started:") || strings.Contains(outConflict, `"status":"success"`) {
					t.Fatalf("expected conflict rejection on different file while first deploy active, got %q", outConflict)
				}

				// 编辑器此时尝试修改并部署同项目（或以 blue 为名创建）：也必须被二进制状态错误拦截，不得写盘
				recEdConflict := startCreateUpWS(t, tc.dirName, cFile, "services:\n  hijack:\n    image: busybox:1.36\n", true)
				_ = waitDone(t, recEdConflict)
				if ecs := extractStatusFrames(recEdConflict); len(ecs) != 1 || ecs[0].msgType != websocket.BinaryMessage || ecs[0].status != "error" {
					_ = os.Remove(gate1)
					_ = waitDone(t, rec1)
					waitHubCountZero(t)
					t.Fatalf("expected editor binary conflict error during first deploy window, got %#v", ecs)
				}

				// 同卡片再次点击（此时 ResolveManagedCardPlan 已能发现 RuntimeProjectName="blue"）：允许复用同一活跃流
				rec2 := startWS(t, "up", tc.dirName, cFile)
				time.Sleep(50 * time.Millisecond)
				if got := mgr.GetHubCount(); got != 1 {
					_ = os.Remove(gate1)
					_ = waitDone(t, rec1)
					_ = waitDone(t, rec2)
					waitHubCountZero(t)
					t.Fatalf("expected single active hub when second click resolves runtime name blue, got %d", got)
				}

				_ = os.Remove(gate1)
				_ = waitDone(t, rec1)
				_ = waitDone(t, rec2)
				s1 := extractStatusFrames(rec1)
				s2 := extractStatusFrames(rec2)
				if len(s1) != 1 || s1[0].status != "success" || len(s2) != 1 || s2[0].status != "success" {
					t.Fatalf("expected both connections on first deploy to succeed, s1=%#v s2=%#v", s1, s2)
				}
				waitHubCountZero(t)

				callsFirst := readRecordedDockerCalls(t, logPath)
				if len(callsFirst) != 2 {
					t.Fatalf("expected exact 2 docker calls (1 up + 1 ps) during overlapping discovery window, got %d: %#v", len(callsFirst), callsFirst)
				}
				wantFirstUp := []string{"compose", "-f", cFile, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
				wantFirstPs := []string{"compose", "-f", cFile, "ps"}
				if callsFirst[0].pwd != projDir || !slices.Equal(callsFirst[0].args, wantFirstUp) {
					t.Fatalf("first up call must omit dir -p: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", callsFirst[0].pwd, callsFirst[0].args, projDir, wantFirstUp)
				}
				if callsFirst[1].pwd != projDir || !slices.Equal(callsFirst[1].args, wantFirstPs) {
					t.Fatalf("first ps call must omit dir -p: got=(pwd=%q args=%#v) want=(pwd=%q args=%#v)", callsFirst[1].pwd, callsFirst[1].args, projDir, wantFirstPs)
				}

				// 首次部署结束后再次发起同卡片部署，应正常执行并带上发现后的 -p blue
				rec3 := startWS(t, "up", tc.dirName, cFile)
				_ = waitDone(t, rec3)
				if s3 := extractStatusFrames(rec3); len(s3) != 1 || s3[0].status != "success" {
					t.Fatalf("expected post-completion redeploy to succeed, got %#v", s3)
				}
				callsAll := readRecordedDockerCalls(t, logPath)
				if len(callsAll) != 4 {
					t.Fatalf("expected 4 total calls after sequential redeploy, got %d: %#v", len(callsAll), callsAll)
				}
				wantSecondUp := []string{"compose", "-p", "blue", "-f", cFile, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
				if !slices.Equal(callsAll[2].args, wantSecondUp) {
					t.Fatalf("post-discovery up call mismatch: got=%#v want=%#v", callsAll[2].args, wantSecondUp)
				}
				waitHubCountZero(t)
			})
		}
	})

	// 7. Item 3: 日志订阅流使用独立命名空间，不得占用 compose-up-dual 对应 dual 的部署槽位
	t.Run("active logs subscription on compose-up-dual does not block deployment of dual", func(t *testing.T) {
		setRuntimeProjects(nil)
		if err := os.WriteFile(composeYaml, []byte("services:\n  web:\n    image: nginx:1.25\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		logProjDir := filepath.Join(resolvedAppDir, "compose-up-dual")
		if err := os.MkdirAll(logProjDir, 0o755); err != nil {
			t.Fatal(err)
		}
		logProjCompose := filepath.Join(logProjDir, "compose.yaml")
		if err := os.WriteFile(logProjCompose, []byte("services:\n  logger:\n    image: busybox:1.36\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		logPath := filepath.Join(t.TempDir(), "logs-and-up.log")
		gateDir := t.TempDir()
		logsGate := filepath.Join(gateDir, "block_logs")
		if err := os.WriteFile(logsGate, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(logsGate)
		installGatedFakeDocker(t, logPath, gateDir)

		recLogs := startWS(t, "logs", "compose-up-dual", logProjCompose)
		waitContains(t, recLogs, "started:")

		recUp := startWS(t, "up", "dual", composeYaml)
		outUp := waitDone(t, recUp)
		if upStatuses := extractStatusFrames(recUp); len(upStatuses) != 1 || upStatuses[0].status != "success" {
			t.Fatalf("expected dual up to succeed while compose-up-dual logs stream is open, got statuses=%#v out=%q", upStatuses, outUp)
		}

		_ = os.Remove(logsGate)
		waitContains(t, recLogs, "finished:")
		_ = waitDone(t, recLogs)
		waitHubCountZero(t)

		calls := readRecordedDockerCalls(t, logPath)
		var logsCalls, upCalls, psCalls int
		for _, c := range calls {
			switch {
			case slices.Contains(c.args, "logs"):
				logsCalls++
			case slices.Contains(c.args, "up"):
				upCalls++
			case slices.Contains(c.args, "ps"):
				psCalls++
			}
		}
		if logsCalls != 1 || upCalls != 1 || psCalls != 1 {
			t.Fatalf("expected 1 logs, 1 up, 1 ps call, got logs=%d up=%d ps=%d (%#v)", logsCalls, upCalls, psCalls, calls)
		}
	})

	// 8. 跨目录运行时项目冲突（shared 目录 vs 已运行项目 shared 指向 blue/compose.yaml）必须在写盘前拦截，不得写入 YAML
	t.Run("cross-directory runtime deploy conflict rejects before writing yaml and releases on error", func(t *testing.T) {
		defer setRuntimeProjects(nil)

		blueDir := filepath.Join(resolvedAppDir, "blue")
		sharedDir := filepath.Join(resolvedAppDir, "shared")
		_ = os.RemoveAll(sharedDir)
		if err := os.MkdirAll(blueDir, 0o755); err != nil {
			t.Fatal(err)
		}
		blueCompose := filepath.Join(blueDir, "compose.yaml")
		origBlue := "services:\n  app:\n    image: busybox:1\n"
		updatedBlue := "services:\n  app:\n    image: busybox:2\n"
		if err := os.WriteFile(blueCompose, []byte(origBlue), 0o644); err != nil {
			t.Fatal(err)
		}

		logPath := filepath.Join(t.TempDir(), "cross-dir-conflict.log")
		installGatedFakeDocker(t, logPath, t.TempDir())

		firstStarted := make(chan struct{})
		releaseFirst := make(chan struct{})
		var listerCalls atomic.Int32
		setRuntimeLister(func(ctx context.Context) ([]composecli.ListOutput, error) {
			if listerCalls.Add(1) == 1 {
				close(firstStarted)
				<-releaseFirst
			}
			return []composecli.ListOutput{{
				Name:        "shared",
				Status:      "running(1)",
				ConfigFiles: blueCompose,
			}}, nil
		})

		// 第一个编辑器请求以 dir=shared 发起保存并部署，在运行时查询阶段暂停
		recShared := startCreateUpWS(t, "shared", "", "services:\n  shared:\n    image: alpine:3.20\n", true)
		select {
		case <-firstStarted:
		case <-time.After(3 * time.Second):
			close(releaseFirst)
			t.Fatal("timed out waiting for first runtime lister call")
		}

		// 第二个编辑器请求尝试把 blue/compose.yaml 改为 busybox:2，解析到运行时项目 shared 后必须在写盘前被拦截
		recBlueConflict := startCreateUpWS(t, "blue", blueCompose, updatedBlue, true)
		outBlueConflict := waitDone(t, recBlueConflict)
		conflictStatuses := extractStatusFrames(recBlueConflict)
		if len(conflictStatuses) != 1 || conflictStatuses[0].msgType != websocket.BinaryMessage || conflictStatuses[0].status != "error" || !strings.Contains(conflictStatuses[0].message, "正在部署中") {
			close(releaseFirst)
			_ = waitDone(t, recShared)
			waitHubCountZero(t)
			t.Fatalf("expected binary conflict error for blue while shared is active, got statuses=%#v out=%q", conflictStatuses, outBlueConflict)
		}

		// 核心断言：blue/compose.yaml 仍逐字节等于原文，未写入 busybox:2，且产生零次 docker 调用
		if got, err := os.ReadFile(blueCompose); err != nil || string(got) != origBlue {
			close(releaseFirst)
			_ = waitDone(t, recShared)
			waitHubCountZero(t)
			t.Fatalf("blue/compose.yaml mutated despite deploy conflict: err=%v got=%q want=%q", err, string(got), origBlue)
		}
		if _, err := os.Stat(sharedDir); !os.IsNotExist(err) {
			close(releaseFirst)
			_ = waitDone(t, recShared)
			waitHubCountZero(t)
			t.Fatalf("shared directory should not be created before write phase completes, stat err=%v", err)
		}
		if calls := readRecordedDockerCalls(t, logPath); len(calls) != 0 {
			close(releaseFirst)
			_ = waitDone(t, recShared)
			waitHubCountZero(t)
			t.Fatalf("expected 0 docker calls during conflict window, got %#v", calls)
		}

		// 放行第一个请求并确认其正常结束释放槽位
		close(releaseFirst)
		_ = waitDone(t, recShared)
		if ss := extractStatusFrames(recShared); len(ss) != 1 || ss[0].status != "error" {
			t.Fatalf("expected shared request referencing foreign runtime blue file to fail cleanly, got %#v", ss)
		}
		if got, err := os.ReadFile(blueCompose); err != nil || string(got) != origBlue {
			t.Fatalf("blue/compose.yaml changed after shared finished: err=%v got=%q", err, string(got))
		}
		waitHubCountZero(t)

		// 覆盖写盘失败时的槽位释放出口，随后应能再次成功提交
		if os.Geteuid() != 0 {
			if err := os.Chmod(blueCompose, 0o444); err != nil {
				t.Fatal(err)
			}
			recWriteFail := startCreateUpWS(t, "blue", blueCompose, updatedBlue, true)
			_ = waitDone(t, recWriteFail)
			if wf := extractStatusFrames(recWriteFail); len(wf) != 1 || wf[0].status != "error" {
				_ = os.Chmod(blueCompose, 0o644)
				t.Fatalf("expected error status on read-only write failure, got %#v", wf)
			}
			if err := os.Chmod(blueCompose, 0o644); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(blueCompose); string(got) != origBlue {
				t.Fatalf("blue/compose.yaml modified on read-only failure: %q", string(got))
			}
			waitHubCountZero(t)
		}

		// 第一个请求（及写失败请求）释放槽位后，单独重试 blue 应成功写入 busybox:2 并执行一次 up + ps
		recRetry := startCreateUpWS(t, "blue", blueCompose, updatedBlue, true)
		outRetry := waitDone(t, recRetry)
		if rs := extractStatusFrames(recRetry); len(rs) != 1 || rs[0].status != "success" {
			t.Fatalf("expected blue retry to succeed, got %#v out=%q", rs, outRetry)
		}
		if got, err := os.ReadFile(blueCompose); err != nil || string(got) != updatedBlue {
			t.Fatalf("expected blue/compose.yaml updated to busybox:2, got=%q err=%v", string(got), err)
		}
		callsAfter := readRecordedDockerCalls(t, logPath)
		wantUp := []string{"compose", "-p", "shared", "-f", blueCompose, "--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"}
		wantPs := []string{"compose", "-p", "shared", "-f", blueCompose, "ps"}
		if len(callsAfter) != 2 || !slices.Equal(callsAfter[0].args, wantUp) || !slices.Equal(callsAfter[1].args, wantPs) {
			t.Fatalf("unexpected docker calls after retry: %#v", callsAfter)
		}
		waitHubCountZero(t)
	})
}

func TestNestedRuntimeProjectLogsAndLifecycleAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpRoot := t.TempDir()
	appDir := filepath.Join(tmpRoot, "apps")
	teamDir := filepath.Join(appDir, "team")
	siteDir := filepath.Join(teamDir, "site")
	otherDir := filepath.Join(teamDir, "other")
	for _, d := range []string{siteDir, otherDir} {
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

	nestedCompose := filepath.Join(resolvedAppDir, "team", "site", "compose.yaml")
	nestedOtherCompose := filepath.Join(resolvedAppDir, "team", "other", "compose.yaml")
	rootCompose := filepath.Join(resolvedAppDir, "compose.yaml")
	origNestedYaml := "services:\n  web:\n    image: nginx:1.25\n"
	for _, f := range []string{nestedCompose, nestedOtherCompose, rootCompose} {
		if err := os.WriteFile(f, []byte(origNestedYaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	symRootFile := filepath.Join(resolvedAppDir, "sym-root.yaml")
	if err := os.Symlink(rootCompose, symRootFile); err != nil {
		t.Fatal(err)
	}
	symNestedFile := filepath.Join(resolvedAppDir, "team", "site", "sym.yaml")
	if err := os.Symlink(nestedCompose, symNestedFile); err != nil {
		t.Fatal(err)
	}
	symTeamDir := filepath.Join(resolvedAppDir, "sym-team")
	if err := os.Symlink(filepath.Join(resolvedAppDir, "team"), symTeamDir); err != nil {
		t.Fatal(err)
	}

	cli := &composecli.Client{
		RuntimeProjectsLister: func(ctx context.Context) ([]composecli.ListOutput, error) {
			return []composecli.ListOutput{
				{Name: "site", Status: "running(1)", ConfigFiles: nestedCompose},
				{Name: "other", Status: "running(1)", ConfigFiles: nestedOtherCompose},
				{Name: "apps", Status: "running(1)", ConfigFiles: rootCompose},
			}, nil
		},
	}
	mgr := wsstream.NewStreamManager[[]byte]()
	t.Cleanup(mgr.Close)

	srv := &Server{
		logger:             zap.NewNop(),
		composeClient:      cli,
		streamManagerBytes: mgr,
	}
	router := gin.New()
	router.GET("/api/v1/compose/logs/:projectName/ws", srv.handleComposeLogsWebSocket())
	router.POST("/api/v1/compose/stop", srv.handleStopComposeProject())
	router.GET("/api/v1/compose/:projectName/yaml", srv.handleGetProjectYaml())
	router.GET("/api/v1/compose/up/:projectName/ws", srv.handleComposeUpExistingWebSocket())
	router.GET("/api/v1/compose/pull/:projectName/ws", srv.handleComposePullWebSocket())
	router.GET("/api/v1/compose/create-and-up/ws", srv.handleComposeCreateAndUpWebSocket())

	httpSrv := httptest.NewServer(router)
	defer httpSrv.Close()
	wsBase := "ws" + strings.TrimPrefix(httpSrv.URL, "http")

	waitHubCountZero := func(t *testing.T) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for mgr.GetHubCount() != 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if got := mgr.GetHubCount(); got != 0 {
			t.Fatalf("expected hub count 0, got %d", got)
		}
	}

	// 1. 允许对嵌套运行时项目与 APP_PATH 根目录运行时项目打开日志 WebSocket 与执行 stop 生命周期操作
	okLogPath := filepath.Join(t.TempDir(), "nested-ok.log")
	installFakeDockerForCardTest(t, okLogPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	resolvedWD, err := filepath.EvalSymlinks(wd)
	if err != nil {
		resolvedWD = filepath.Clean(wd)
	}

	positiveCases := []struct {
		name        string
		composeFile string
	}{
		{"site", nestedCompose},
		{"apps", rootCompose},
	}
	var allLogFrames strings.Builder
	for idx, tc := range positiveCases {
		wantCallsAfterLogs := idx*2 + 1
		logsURL := wsBase + "/api/v1/compose/logs/" + url.PathEscape(tc.name) + "/ws?composeFile=" + url.QueryEscape(tc.composeFile)
		logsConn, _, derr := websocket.DefaultDialer.Dial(logsURL, nil)
		if derr != nil {
			t.Fatalf("dial logs ws (%s) failed: %v", tc.name, derr)
		}
		_ = logsConn.SetReadDeadline(time.Now().Add(4 * time.Second))
		for {
			_, msg, rerr := logsConn.ReadMessage()
			if len(msg) > 0 {
				allLogFrames.Write(msg)
			}
			if calls := readRecordedDockerCalls(t, okLogPath); len(calls) >= wantCallsAfterLogs {
				_ = logsConn.Close()
				break
			}
			if rerr != nil {
				_ = logsConn.Close()
				break
			}
		}
		waitHubCountZero(t)

		stopBody := `{"name":` + mustJSONString(t, tc.name) + `,"composeFile":` + mustJSONString(t, tc.composeFile) + `,"status":"running"}`
		stopReq, _ := http.NewRequest(http.MethodPost, httpSrv.URL+"/api/v1/compose/stop", bytes.NewBufferString(stopBody))
		stopReq.Header.Set("Content-Type", "application/json")
		stopResp, derr := http.DefaultClient.Do(stopReq)
		if derr != nil {
			t.Fatalf("POST /compose/stop (%s) failed: %v", tc.name, derr)
		}
		var stopRes struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		decErr := json.NewDecoder(stopResp.Body).Decode(&stopRes)
		_ = stopResp.Body.Close()
		if decErr != nil {
			t.Fatalf("decode stop response (%s) failed: %v", tc.name, decErr)
		}
		if stopResp.StatusCode != http.StatusOK || stopRes.Code != 0 {
			t.Fatalf("expected stop success (%s, code=0), got status=%d res=%#v", tc.name, stopResp.StatusCode, stopRes)
		}
	}

	okCalls := readRecordedDockerCalls(t, okLogPath)
	wantCalls := [][]string{
		{"compose", "--env-file", os.DevNull, "-p", "site", "--ansi", "always", "logs", "--follow", "--timestamps", "--tail=500"},
		{"compose", "--env-file", os.DevNull, "-p", "site", "stop"},
		{"compose", "--env-file", os.DevNull, "-p", "apps", "--ansi", "always", "logs", "--follow", "--timestamps", "--tail=500"},
		{"compose", "--env-file", os.DevNull, "-p", "apps", "stop"},
	}
	if len(okCalls) != len(wantCalls) {
		t.Fatalf("unexpected docker call count (%d != %d): %#v (frames=%q)", len(okCalls), len(wantCalls), okCalls, allLogFrames.String())
	}
	for i, wantArgs := range wantCalls {
		if !slices.Equal(okCalls[i].args, wantArgs) {
			t.Fatalf("call[%d] args = %#v, want %#v", i, okCalls[i].args, wantArgs)
		}
		callPwd, perr := filepath.EvalSymlinks(okCalls[i].pwd)
		if perr != nil {
			callPwd = filepath.Clean(okCalls[i].pwd)
		}
		if callPwd != resolvedWD || callPwd == resolvedAppDir || callPwd == siteDir {
			t.Fatalf("call[%d] pwd = %q (resolved %q), want process wd %q (not %q or %q)", i, okCalls[i].pwd, callPwd, resolvedWD, resolvedAppDir, siteDir)
		}
	}

	// 2. 负例与特权入口隔离：拒绝未知项目、名称/路径不匹配、符号链接、..穿越；编辑/YAML 读取/卡片 up&pull 仍拒绝嵌套与根目录路径并执行 0 次 docker 命令
	rejectLogPath := filepath.Join(t.TempDir(), "nested-reject.log")
	installFakeDockerForCardTest(t, rejectLogPath)

	postStopExpectError := func(name, composeFile string) {
		t.Helper()
		payload := map[string]string{"name": name, "composeFile": composeFile, "status": "running"}
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, httpSrv.URL+"/api/v1/compose/stop", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, derr := http.DefaultClient.Do(req)
		if derr != nil {
			t.Fatalf("stop request (%s, %s) failed: %v", name, composeFile, derr)
		}
		defer resp.Body.Close()
		var res struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || res.Code == 0 {
			t.Fatalf("expected error code for (%s, %s), got res=%#v err=%v", name, composeFile, res, err)
		}
	}

	traversalNested := resolvedAppDir + string(filepath.Separator) + "team" + string(filepath.Separator) + ".." + string(filepath.Separator) + "team" + string(filepath.Separator) + "site" + string(filepath.Separator) + "compose.yaml"
	for _, bad := range []struct {
		name        string
		composeFile string
	}{
		{"unknown", nestedCompose},
		{"other", nestedCompose},
		{"site", nestedOtherCompose},
		{"site", symNestedFile},
		{"site", filepath.Join(symTeamDir, "site", "compose.yaml")},
		{"site", traversalNested},
		{"site", rootCompose},
		{"unknown", rootCompose},
		{"apps", symRootFile},
	} {
		postStopExpectError(bad.name, bad.composeFile)

		badLogConn, _, derr := websocket.DefaultDialer.Dial(wsBase+"/api/v1/compose/logs/"+url.PathEscape(bad.name)+"/ws?composeFile="+url.QueryEscape(bad.composeFile), nil)
		if derr != nil {
			t.Fatalf("dial bad logs (%#v) failed: %v", bad, derr)
		}
		_ = badLogConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, _ := badLogConn.ReadMessage()
		_ = badLogConn.Close()
		if len(msg) == 0 {
			t.Fatalf("expected error frame for bad logs (%#v)", bad)
		}
	}

	// YAML 读取、卡片 up/pull 与编辑器保存均必须继续拒绝嵌套文件与 APP_PATH 根目录文件
	for _, unmanaged := range []struct {
		name        string
		composeFile string
	}{
		{"site", nestedCompose},
		{"apps", rootCompose},
	} {
		yamlResp, yerr := http.Get(httpSrv.URL + "/api/v1/compose/" + url.PathEscape(unmanaged.name) + "/yaml?composeFile=" + url.QueryEscape(unmanaged.composeFile))
		if yerr != nil {
			t.Fatal(yerr)
		}
		var yamlBody struct {
			Code int `json:"code"`
		}
		decErr := json.NewDecoder(yamlResp.Body).Decode(&yamlBody)
		_ = yamlResp.Body.Close()
		if decErr != nil || yamlBody.Code == 0 {
			t.Fatalf("expected GET /yaml error for unmanaged path (%#v), got %#v err=%v", unmanaged, yamlBody, decErr)
		}

		for _, op := range []string{"up", "pull"} {
			conn, _, derr := websocket.DefaultDialer.Dial(wsBase+"/api/v1/compose/"+op+"/"+url.PathEscape(unmanaged.name)+"/ws?composeFile="+url.QueryEscape(unmanaged.composeFile), nil)
			if derr != nil {
				t.Fatal(derr)
			}
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, msg, _ := conn.ReadMessage()
			_ = conn.Close()
			if len(msg) == 0 {
				t.Fatalf("expected error frame on %s for unmanaged compose path %#v", op, unmanaged)
			}
		}

		edConn, _, derr := websocket.DefaultDialer.Dial(wsBase+"/api/v1/compose/create-and-up/ws", nil)
		if derr != nil {
			t.Fatal(derr)
		}
		_ = edConn.WriteJSON(map[string]any{
			"name":        unmanaged.name,
			"composeFile": unmanaged.composeFile,
			"yamlContent": "services:\n  web:\n    image: busybox:1.36\n",
			"force":       true,
		})
		_ = edConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var edOut strings.Builder
		for {
			_, chunk, rerr := edConn.ReadMessage()
			if len(chunk) > 0 {
				edOut.Write(chunk)
			}
			if rerr != nil {
				break
			}
		}
		_ = edConn.Close()
		if !strings.Contains(edOut.String(), `"status":"error"`) || strings.Contains(edOut.String(), `"status":"success"`) {
			t.Fatalf("expected error status frame when editing unmanaged file (%#v), got %q", unmanaged, edOut.String())
		}

		if got, _ := os.ReadFile(unmanaged.composeFile); string(got) != origNestedYaml {
			t.Fatalf("unmanaged compose file %s modified despite rejections: %q", unmanaged.composeFile, string(got))
		}
	}
	if calls := readRecordedDockerCalls(t, rejectLogPath); len(calls) != 0 {
		t.Fatalf("expected 0 docker calls on rejected unmanaged operations, got %#v", calls)
	}
	waitHubCountZero(t)
}

func mustJSONString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
