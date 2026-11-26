package dockercli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/moby/moby/client"
)

func TestRegistryAuthAndMirrorPullTrimWhitespace(t *testing.T) {
	origEnv := conf.EnvCfg
	origCfg := config.Get()
	dir := t.TempDir()
	testEnv := *origEnv
	testEnv.CONFIG_PATH = dir
	testEnv.CONFIG_FILE = "config.yaml"
	conf.EnvCfg = &testEnv
	cfgPath := filepath.Join(dir, "config.yaml")
	t.Cleanup(func() {
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
		conf.EnvCfg = origEnv
	})

	var pulledFromImage atomic.Value
	var taggedTarget atomic.Value
	var taggedTag atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			pulledFromImage.Store(r.URL.Query().Get("fromImage"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/tag"):
			taggedTarget.Store(r.URL.Query().Get("repo"))
			taggedTag.Store(r.URL.Query().Get("tag"))
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/images/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	yamlCfg := `registry:
  auth:
    - host: ghcr.io
      username: octocat
      token: ghp_secret
  mirrors:
    - name: m1
      url: https://mirror.example.com
      enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(yamlCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(cfgPath); err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if authHeader := registryAuthForRef(" ghcr.io/org/app:1.0 "); authHeader == "" {
		t.Fatalf("expected non-empty registryAuthForRef for whitespace-padded ref")
	}

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()
	cli := &Client{docker: dockerCli}

	if err := cli.ImagePull(context.Background(), " nginx:1.25 "); err != nil {
		t.Fatalf("ImagePull padded ref via mirror failed: %v", err)
	}
	gotPull, _ := pulledFromImage.Load().(string)
	if !strings.HasPrefix(gotPull, "mirror.example.com/library/nginx") {
		t.Fatalf("expected mirror fromImage, got %q", gotPull)
	}
	gotRepo, _ := taggedTarget.Load().(string)
	gotTag, _ := taggedTag.Load().(string)
	if gotRepo != "docker.io/library/nginx" || gotTag != "1.25" {
		t.Fatalf("expected trimmed retag repo=docker.io/library/nginx tag=1.25, got repo=%q tag=%q", gotRepo, gotTag)
	}
}
