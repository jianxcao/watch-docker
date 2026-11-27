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
