package dockercli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/distribution/reference"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/moby/moby/api/pkg/authconfig"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

type recordedEnginePull struct {
	fromImage string
	tag       string
	rawAuth   string
	auth      registrytypes.AuthConfig
}

func isAnonymousEngineAuth(cfg registrytypes.AuthConfig) bool {
	return cfg.Username == "" &&
		cfg.Password == "" &&
		cfg.Auth == "" &&
		cfg.IdentityToken == "" &&
		cfg.RegistryToken == ""
}

func assertServerAddressMatchesFromImage(t *testing.T, pull recordedEnginePull, wantScheme string) {
	t.Helper()
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(pull.fromImage))
	if err != nil {
		t.Fatalf("parse fromImage %q failed: %v", pull.fromImage, err)
	}
	wantHost := reference.Domain(named)
	parsedAddr, err := url.Parse(strings.TrimSpace(pull.auth.ServerAddress))
	if err != nil {
		t.Fatalf("parse ServerAddress %q failed: %v", pull.auth.ServerAddress, err)
	}
	if parsedAddr.Scheme != wantScheme || parsedAddr.Host != wantHost {
		t.Fatalf("ServerAddress mismatch for fromImage=%q: got=%q (scheme=%q host=%q) want=(scheme=%q host=%q)",
			pull.fromImage, pull.auth.ServerAddress, parsedAddr.Scheme, parsedAddr.Host, wantScheme, wantHost)
	}
}

func TestImagePullMirrorCredentialOrigin(t *testing.T) {
	origEnv := conf.EnvCfg
	origCfg := config.Get()
	testEnv := *origEnv
	conf.EnvCfg = &testEnv
	t.Cleanup(func() {
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
		conf.EnvCfg = origEnv
	})

	loadPullCfg := func(t *testing.T, yamlCfg string) {
		t.Helper()
		dir := t.TempDir()
		conf.EnvCfg.CONFIG_PATH = dir
		conf.EnvCfg.CONFIG_FILE = "config.yaml"
		cfgPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfgPath, []byte(yamlCfg), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.Load(cfgPath); err != nil {
			t.Fatalf("load config failed: %v", err)
		}
	}

	setupFakeEngine := func(t *testing.T, failFrom func(string) bool) (*Client, func() []recordedEnginePull, func()) {
		t.Helper()
		var mu sync.Mutex
		var pulls []recordedEnginePull

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
				fromImage := r.URL.Query().Get("fromImage")
				tag := r.URL.Query().Get("tag")
				rawAuth := r.Header.Get("X-Registry-Auth")
				var decoded registrytypes.AuthConfig
				if rawAuth != "" {
					cfgPtr, err := authconfig.Decode(rawAuth)
					if err != nil {
						t.Errorf("decode X-Registry-Auth failed (%q): %v", rawAuth, err)
					} else if cfgPtr != nil {
						decoded = *cfgPtr
					}
				}
				mu.Lock()
				pulls = append(pulls, recordedEnginePull{
					fromImage: fromImage,
					tag:       tag,
					rawAuth:   rawAuth,
					auth:      decoded,
				})
				mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				if failFrom != nil && failFrom(fromImage) {
					_, _ = w.Write([]byte(`{"error":"mirror down","errorDetail":{"message":"mirror down"}}` + "\n"))
					return
				}
				_, _ = w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/tag"):
				w.WriteHeader(http.StatusCreated)
			case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/images/"):
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(srv.Close)

		dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
		if err != nil {
			t.Fatalf("create docker client: %v", err)
		}
		t.Cleanup(func() { _ = dockerCli.Close() })

		snapshot := func() []recordedEnginePull {
			mu.Lock()
			defer mu.Unlock()
			return append([]recordedEnginePull(nil), pulls...)
		}
		reset := func() {
			mu.Lock()
			pulls = nil
			mu.Unlock()
		}
		return &Client{docker: dockerCli}, snapshot, reset
	}

	platform := &ocispec.Platform{OS: "linux", Architecture: "amd64"}
	ctx := context.Background()

	// 1. 同地址匿名 HTTP mirror 与带账号 HTTPS（两种顺序），HTTP mirror 均为匿名
	anonOrders := []struct {
		name string
		cfg  string
	}{
		{
			name: "anon_http_then_https_cred",
			cfg: `registry:
  auth:
    - host: http://registry.example:5000
      username: ""
      token: ""
    - host: https://registry.example:5000
      username: https-user
      token: https-secret
  mirrors:
    - name: m-http
      url: http://registry.example:5000
      enabled: true
`,
		},
		{
			name: "https_cred_then_anon_http",
			cfg: `registry:
  auth:
    - host: https://registry.example:5000
      username: https-user
      token: https-secret
    - host: http://registry.example:5000
      username: ""
      token: ""
  mirrors:
    - name: m-http
      url: http://registry.example:5000
      enabled: true
`,
		},
	}

	for _, tc := range anonOrders {
		t.Run("anon http mirror plus credentialed https/"+tc.name, func(t *testing.T) {
			loadPullCfg(t, tc.cfg)
			cli, snapshot, _ := setupFakeEngine(t, nil)
			if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
				t.Fatalf("ImagePullWithPlatform failed: %v", err)
			}
			pulls := snapshot()
			if len(pulls) != 1 {
				t.Fatalf("expected 1 pull call, got %#v", pulls)
			}
			if pulls[0].fromImage != "registry.example:5000/library/busybox" || pulls[0].tag != "latest" {
				t.Fatalf("unexpected mirror pull target: %#v", pulls[0])
			}
			if !isAnonymousEngineAuth(pulls[0].auth) {
				t.Fatalf("expected anonymous auth for http mirror, got %#v", pulls[0].auth)
			}
		})
	}

	// 2. 同地址的 HTTP、HTTPS 使用不同账号（两种顺序），每种 mirror 只取同协议账号
	dualOrders := []struct {
		name     string
		authYAML string
	}{
		{
			name: "http_then_https",
			authYAML: `    - host: http://registry.example:5000
      username: http-user
      token: http-secret
    - host: https://registry.example:5000
      username: https-user
      token: https-secret`,
		},
		{
			name: "https_then_http",
			authYAML: `    - host: https://registry.example:5000
      username: https-user
      token: https-secret
    - host: http://registry.example:5000
      username: http-user
      token: http-secret`,
		},
	}
	for _, ord := range dualOrders {
		for _, mirrorScheme := range []string{"http", "https"} {
			wantUser, wantPass := "http-user", "http-secret"
			if mirrorScheme == "https" {
				wantUser, wantPass = "https-user", "https-secret"
			}
			t.Run("dual credentials/"+ord.name+"/mirror_"+mirrorScheme, func(t *testing.T) {
				cfg := "registry:\n  auth:\n" + ord.authYAML + "\n  mirrors:\n    - name: m\n      url: " + mirrorScheme + "://registry.example:5000\n      enabled: true\n"
				loadPullCfg(t, cfg)
				cli, snapshot, _ := setupFakeEngine(t, nil)
				if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
					t.Fatalf("ImagePullWithPlatform failed: %v", err)
				}
				pulls := snapshot()
				if len(pulls) != 1 || pulls[0].fromImage != "registry.example:5000/library/busybox" {
					t.Fatalf("unexpected pull calls: %#v", pulls)
				}
				if pulls[0].auth.Username != wantUser || pulls[0].auth.Password != wantPass {
					t.Fatalf("mirror=%s:// expected (%s, %s), got %#v", mirrorScheme, wantUser, wantPass, pulls[0].auth)
				}
				assertServerAddressMatchesFromImage(t, pulls[0], mirrorScheme)
			})
		}
	}

	// 3. 同一主机不同端口独立匹配，且保留 mirror 路径前缀；默认端口互认
	t.Run("port isolation, mirror path prefix, and default port equivalence", func(t *testing.T) {
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example:8443
      username: user-8443
      token: pass-8443
    - host: https://registry.example:9443
      username: user-9443
      token: pass-9443
    - host: https://registry.example
      username: user-default
      token: pass-default
  mirrors:
    - name: prefixed-8443
      url: https://registry.example:8443/dockerhub/
      enabled: true
`)
		cli, snapshot, reset := setupFakeEngine(t, nil)
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("pull prefixed 8443 mirror failed: %v", err)
		}
		pulls := snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "registry.example:8443/dockerhub/library/busybox" || pulls[0].tag != "latest" {
			t.Fatalf("expected path-prefixed mirror ref, got %#v", pulls)
		}
		if pulls[0].auth.Username != "user-8443" || pulls[0].auth.Password != "pass-8443" {
			t.Fatalf("expected user-8443, got %#v", pulls[0].auth)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		// 未配置账号的 :7443 端口保持匿名
		reset()
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example:8443
      username: user-8443
      token: pass-8443
  mirrors:
    - name: unconfigured-7443
      url: https://registry.example:7443/dockerhub
      enabled: true
`)
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("pull unconfigured 7443 mirror failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "registry.example:7443/dockerhub/library/busybox" || !isAnonymousEngineAuth(pulls[0].auth) {
			t.Fatalf("expected anonymous auth for unconfigured port 7443, got %#v", pulls)
		}

		// 默认 443 端口互认（auth 无显式端口，mirror 写 :443；反之亦然）
		reset()
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example
      username: user-default
      token: pass-default
  mirrors:
    - name: explicit-443
      url: https://registry.example:443/hub-proxy/
      enabled: true
`)
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("pull explicit-443 mirror failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "registry.example:443/hub-proxy/library/busybox" || pulls[0].auth.Username != "user-default" {
			t.Fatalf("expected user-default matched on :443 mirror, got %#v", pulls)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		// 反向互认：auth 写 :443，mirror 省略端口时 ServerAddress 随 mirror 引用去掉 :443
		reset()
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example:443
      username: user-443
      token: pass-443
  mirrors:
    - name: omitted-443
      url: https://registry.example/hub-proxy/
      enabled: true
`)
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("pull omitted-port mirror with :443 auth failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "registry.example/hub-proxy/library/busybox" || pulls[0].auth.Username != "user-443" {
			t.Fatalf("expected user-443 matched on omitted-port mirror, got %#v", pulls)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		// http 与 https 在同主机默认端口也不混用
		reset()
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example
      username: user-https
      token: pass-https
  mirrors:
    - name: plain-http
      url: http://registry.example/hub-proxy
      enabled: true
`)
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("pull http mirror with https-only default auth failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || !isAnonymousEngineAuth(pulls[0].auth) {
			t.Fatalf("expected anonymous auth for http default-port mirror, got %#v", pulls)
		}
	})

	// 4. mirror 拉取失败回退 Docker Hub 时，第一次用 mirror 账号，第二次回退使用 Docker Hub 自身认证配置
	t.Run("mirror failure falls back to docker hub credentials", func(t *testing.T) {
		loadPullCfg(t, `registry:
  auth:
    - host: https://registry.example:5000
      username: mirror-user
      token: mirror-secret
    - host: registry-1.docker.io
      username: hub-user
      token: hub-secret
  mirrors:
    - name: m1
      url: https://registry.example:5000
      enabled: true
`)
		cli, snapshot, _ := setupFakeEngine(t, func(fromImage string) bool {
			return strings.HasPrefix(fromImage, "registry.example:5000/")
		})
		if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
			t.Fatalf("expected fallback to docker hub to succeed, got %v", err)
		}
		pulls := snapshot()
		if len(pulls) != 2 {
			t.Fatalf("expected 2 pull attempts (mirror + hub fallback), got %#v", pulls)
		}
		if pulls[0].fromImage != "registry.example:5000/library/busybox" || pulls[0].auth.Username != "mirror-user" || pulls[0].auth.Password != "mirror-secret" {
			t.Fatalf("first attempt mismatch: %#v", pulls[0])
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")
		if !strings.Contains(pulls[1].fromImage, "busybox") || strings.HasPrefix(pulls[1].fromImage, "registry.example:5000/") {
			t.Fatalf("second attempt should target docker hub, got %#v", pulls[1])
		}
		if pulls[1].auth.Username != "hub-user" || pulls[1].auth.Password != "hub-secret" || pulls[1].auth.ServerAddress != "https://index.docker.io/v1/" {
			t.Fatalf("second attempt expected hub credentials + ServerAddress, got %#v", pulls[1].auth)
		}
	})

	// 5. 无 mirror 时直拉私有 HTTPS 仓库正常使用对应账号；直拉默认端口省略/显式写法保持宿主端口一致；双协议配置含匿名 HTTP 时直拉保持匿名
	t.Run("no mirrors direct private https pull and direct dual-origin isolation", func(t *testing.T) {
		loadPullCfg(t, `registry:
  auth:
    - host: https://priv.example:8443
      username: priv-user
      token: priv-secret
    - host: https://priv.example
      username: priv-def-user
      token: priv-def-secret
    - host: http://dual.example:5000
      username: ""
      token: ""
    - host: https://dual.example:5000
      username: dual-https-user
      token: dual-https-secret
`)
		cli, snapshot, reset := setupFakeEngine(t, nil)
		if err := cli.ImagePullWithPlatform(ctx, "priv.example:8443/team/app:v1", platform, nil); err != nil {
			t.Fatalf("direct private HTTPS pull failed: %v", err)
		}
		pulls := snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "priv.example:8443/team/app" || pulls[0].tag != "v1" {
			t.Fatalf("unexpected direct pull target: %#v", pulls)
		}
		if pulls[0].auth.Username != "priv-user" || pulls[0].auth.Password != "priv-secret" {
			t.Fatalf("expected priv-user on direct private HTTPS pull, got %#v", pulls[0].auth)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		reset()
		if err := cli.ImagePullWithPlatform(ctx, "priv.example:443/team/app:v1", platform, nil); err != nil {
			t.Fatalf("direct private explicit :443 HTTPS pull failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "priv.example:443/team/app" || pulls[0].auth.Username != "priv-def-user" {
			t.Fatalf("expected priv-def-user on direct :443 pull, got %#v", pulls)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		reset()
		loadPullCfg(t, `registry:
  auth:
    - host: https://priv.example:443
      username: priv-443-user
      token: priv-443-secret
`)
		if err := cli.ImagePullWithPlatform(ctx, "priv.example/team/app:v1", platform, nil); err != nil {
			t.Fatalf("direct private omitted :443 HTTPS pull failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || pulls[0].fromImage != "priv.example/team/app" || pulls[0].auth.Username != "priv-443-user" {
			t.Fatalf("expected priv-443-user on direct omitted-port pull, got %#v", pulls)
		}
		assertServerAddressMatchesFromImage(t, pulls[0], "https")

		loadPullCfg(t, `registry:
  auth:
    - host: http://dual.example:5000
      username: ""
      token: ""
    - host: https://dual.example:5000
      username: dual-https-user
      token: dual-https-secret
`)

		reset()
		if err := cli.ImagePullWithPlatform(ctx, "dual.example:5000/team/app:v2", platform, nil); err != nil {
			t.Fatalf("direct dual-origin pull failed: %v", err)
		}
		pulls = snapshot()
		if len(pulls) != 1 || !isAnonymousEngineAuth(pulls[0].auth) {
			t.Fatalf("expected anonymous auth when explicit http:// entry clears/owns direct origin, got %#v", pulls)
		}
	})

	// 6. 单段主机名 mirror 补全默认端口避免被解析为 docker.io；既有 localhost/域名/显式端口/IPv6 形式保持不变；失败回退 Docker Hub 用其自身账号
	t.Run("single-label mirror hostnames, existing authority forms, and fallback", func(t *testing.T) {
		cases := []struct {
			name          string
			yamlCfg       string
			wantFromImage string
			wantUser      string
			wantPass      string
		}{
			{
				name: "https single-label host with cache prefix",
				yamlCfg: `registry:
  auth:
    - host: https://registry
      username: reg-user
      token: reg-secret
  mirrors:
    - name: m-reg-cache
      url: https://registry/cache
      enabled: true
`,
				wantFromImage: "registry:443/cache/library/busybox",
				wantUser:      "reg-user",
				wantPass:      "reg-secret",
			},
			{
				name: "https single-label host without prefix",
				yamlCfg: `registry:
  auth:
    - host: https://registry
      username: reg-user
      token: reg-secret
  mirrors:
    - name: m-reg
      url: https://registry
      enabled: true
`,
				wantFromImage: "registry:443/library/busybox",
				wantUser:      "reg-user",
				wantPass:      "reg-secret",
			},
			{
				name: "http single-label host with cache prefix keeps http cred only",
				yamlCfg: `registry:
  auth:
    - host: https://registry
      username: https-reg-user
      token: https-reg-secret
    - host: http://registry
      username: http-reg-user
      token: http-reg-secret
  mirrors:
    - name: m-http-reg
      url: http://registry/cache
      enabled: true
`,
				wantFromImage: "registry:80/cache/library/busybox",
				wantUser:      "http-reg-user",
				wantPass:      "http-reg-secret",
			},
			{
				name: "localhost authority stays unannotated",
				yamlCfg: `registry:
  auth:
    - host: https://localhost
      username: local-user
      token: local-secret
  mirrors:
    - name: m-local
      url: https://localhost/cache
      enabled: true
`,
				wantFromImage: "localhost/cache/library/busybox",
				wantUser:      "local-user",
				wantPass:      "local-secret",
			},
			{
				name: "dotted domain stays unannotated",
				yamlCfg: `registry:
  auth:
    - host: https://mirror.example.com
      username: fqdn-user
      token: fqdn-secret
  mirrors:
    - name: m-fqdn
      url: https://mirror.example.com/cache
      enabled: true
`,
				wantFromImage: "mirror.example.com/cache/library/busybox",
				wantUser:      "fqdn-user",
				wantPass:      "fqdn-secret",
			},
			{
				name: "explicit port on single-label host stays exact",
				yamlCfg: `registry:
  auth:
    - host: https://registry:5000
      username: port5000-user
      token: port5000-secret
  mirrors:
    - name: m-port
      url: https://registry:5000/cache
      enabled: true
`,
				wantFromImage: "registry:5000/cache/library/busybox",
				wantUser:      "port5000-user",
				wantPass:      "port5000-secret",
			},
			{
				name: "bracketed IPv6 with port stays exact",
				yamlCfg: `registry:
  auth:
    - host: https://[::1]:5000
      username: v6port-user
      token: v6port-secret
  mirrors:
    - name: m-v6port
      url: https://[::1]:5000/cache
      enabled: true
`,
				wantFromImage: "[::1]:5000/cache/library/busybox",
				wantUser:      "v6port-user",
				wantPass:      "v6port-secret",
			},
			{
				name: "bracketed IPv6 without explicit port stays exact",
				yamlCfg: `registry:
  auth:
    - host: https://[::1]
      username: v6def-user
      token: v6def-secret
  mirrors:
    - name: m-v6def
      url: https://[::1]/cache
      enabled: true
`,
				wantFromImage: "[::1]/cache/library/busybox",
				wantUser:      "v6def-user",
				wantPass:      "v6def-secret",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				loadPullCfg(t, tc.yamlCfg)
				cli, snapshot, _ := setupFakeEngine(t, nil)
				if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
					t.Fatalf("ImagePullWithPlatform failed: %v", err)
				}
				pulls := snapshot()
				if len(pulls) != 1 {
					t.Fatalf("expected 1 pull call, got %#v", pulls)
				}
				if pulls[0].fromImage != tc.wantFromImage || pulls[0].tag != "latest" {
					t.Fatalf("target mismatch: got=(fromImage=%q tag=%q) want=(%q, latest)", pulls[0].fromImage, pulls[0].tag, tc.wantFromImage)
				}
				if pulls[0].auth.Username != tc.wantUser || pulls[0].auth.Password != tc.wantPass {
					t.Fatalf("auth mismatch: got=%#v want=(%s, %s)", pulls[0].auth, tc.wantUser, tc.wantPass)
				}
				wantScheme := "https"
				if strings.HasPrefix(tc.wantFromImage, "registry:80/") {
					wantScheme = "http"
				}
				assertServerAddressMatchesFromImage(t, pulls[0], wantScheme)
			})
		}

		t.Run("single-label mirror error falls back to docker hub with hub credentials", func(t *testing.T) {
			loadPullCfg(t, `registry:
  auth:
    - host: https://registry
      username: reg-user
      token: reg-secret
    - host: docker.io
      username: hub-user
      token: hub-secret
  mirrors:
    - name: m-reg
      url: https://registry/cache
      enabled: true
`)
			cli, snapshot, _ := setupFakeEngine(t, func(fromImage string) bool {
				return strings.HasPrefix(fromImage, "registry:443/")
			})
			if err := cli.ImagePullWithPlatform(ctx, "busybox:latest", platform, nil); err != nil {
				t.Fatalf("expected fallback to docker hub to succeed, got %v", err)
			}
			pulls := snapshot()
			if len(pulls) != 2 {
				t.Fatalf("expected 2 pull calls (mirror + hub fallback), got %#v", pulls)
			}
			if pulls[0].fromImage != "registry:443/cache/library/busybox" || pulls[0].tag != "latest" {
				t.Fatalf("first attempt mismatch: %#v", pulls[0])
			}
			if pulls[0].auth.Username != "reg-user" || pulls[0].auth.Password != "reg-secret" {
				t.Fatalf("first attempt auth mismatch: %#v", pulls[0].auth)
			}
			assertServerAddressMatchesFromImage(t, pulls[0], "https")
			if !strings.Contains(pulls[1].fromImage, "busybox") || strings.HasPrefix(pulls[1].fromImage, "registry") {
				t.Fatalf("second attempt should target docker hub, got %#v", pulls[1])
			}
			if pulls[1].auth.Username != "hub-user" || pulls[1].auth.Password != "hub-secret" || pulls[1].auth.ServerAddress != "https://index.docker.io/v1/" {
				t.Fatalf("second attempt hub auth mismatch: %#v", pulls[1].auth)
			}
		})
	})
}
