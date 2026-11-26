package registry

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
)

func TestAllowInsecureFallback(t *testing.T) {
	c := &Client{
		credentials: make(map[string]authn.AuthConfig),
	}

	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "public ipv6 bracketed with port", host: "[2001:4860:4860::8888]:5000", want: false},
		{name: "public ipv6 bare", host: "2001:4860:4860::8888", want: false},
		{name: "loopback ipv6 bracketed with port", host: "[::1]:5000", want: true},
		{name: "ula private ipv6 with port", host: "[fd00::1]:5000", want: true},
		{name: "public ipv4 with port", host: "8.8.8.8:5000", want: false},
		{name: "loopback ipv4 with port", host: "127.0.0.1:5000", want: true},
		{name: "private ipv4 with port", host: "192.168.1.10:5000", want: true},
		{name: "localhost with port", host: "localhost:5000", want: true},
		{name: "single label service host", host: "registry:5000", want: true},
		{name: "local mdns host", host: "nas.local:5000", want: true},
		{name: "public domain ghcr", host: "ghcr.io", want: false},
		{name: "public domain docker hub", host: "index.docker.io", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.allowInsecureFallback(tc.host); got != tc.want {
				t.Fatalf("allowInsecureFallback(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestRateLimitCooldownDeadlineNotExtendedOnMirrorFallback(t *testing.T) {
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

	const officialManifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[]}`
	officialSum := sha256.Sum256([]byte(officialManifest))
	officialDigest := "sha256:" + hex.EncodeToString(officialSum[:])

	const mirrorManifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":2},"layers":[]}`
	mirrorSum := sha256.Sum256([]byte(mirrorManifest))
	mirrorDigest := "sha256:" + hex.EncodeToString(mirrorSum[:])

	var mirrorHealthy atomic.Bool
	var officialReturn429 atomic.Bool
	officialReturn429.Store(true)
	var officialHits atomic.Int32

	mirrorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mirrorHealthy.Load() {
			http.Error(w, "mirror unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", mirrorDigest)
		_, _ = w.Write([]byte(mirrorManifest))
	}))
	defer mirrorSrv.Close()

	officialSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			if officialReturn429.Load() {
				officialHits.Add(1)
				http.Error(w, "toomanyrequests", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/library/nginx/manifests/") {
			if officialReturn429.Load() {
				officialHits.Add(1)
				http.Error(w, "toomanyrequests", http.StatusTooManyRequests)
				return
			}
			officialHits.Add(1)
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", officialDigest)
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write([]byte(officialManifest))
			return
		}
		http.NotFound(w, r)
	}))
	defer officialSrv.Close()

	yamlCfg := "registry:\n  mirrors:\n    - name: test-mirror\n      url: " + mirrorSrv.URL + "\n      enabled: true\n"
	if err := os.WriteFile(cfgPath, []byte(yamlCfg), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(cfgPath); err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	officialAddr := officialSrv.Listener.Addr().String()
	baseTr := officialSrv.Client().Transport.(*http.Transport).Clone()
	if baseTr.TLSClientConfig != nil {
		baseTr.TLSClientConfig = baseTr.TLSClientConfig.Clone()
		baseTr.TLSClientConfig.InsecureSkipVerify = true
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	baseTr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err == nil && (host == "index.docker.io" || host == "registry-1.docker.io") {
			return dialer.DialContext(ctx, network, officialAddr)
		}
		return dialer.DialContext(ctx, network, addr)
	}

	c := New()
	c.httpTransport = baseTr

	q := []DigestQuery{{ImageRef: "nginx:latest", OS: "linux", Arch: "amd64"}}
	res1 := c.GetRemoteDigestsBatchWithPlatforms(context.Background(), q, false, false, 1)[q[0].Key()]
	if res1.ErrType != ErrorTypeRateLimited {
		t.Fatalf("expected initial ErrorTypeRateLimited, got %#v", res1)
	}
	initialHits := officialHits.Load()
	if initialHits == 0 {
		t.Fatalf("expected >= 1 official hit on initial 429, got 0")
	}

	c.rateLimitMu.RLock()
	before := c.rateLimits["index.docker.io"].Until
	c.rateLimitMu.RUnlock()
	if before.IsZero() {
		t.Fatal("expected cooldown deadline recorded for index.docker.io")
	}

	// 冷却期内 mirror 失败两次，截止时间必须严格不变且不请求官方仓库
	for i := 0; i < 2; i++ {
		res := c.GetRemoteDigestsBatchWithPlatforms(context.Background(), q, false, false, 1)[q[0].Key()]
		if res.ErrType != ErrorTypeRateLimited {
			t.Fatalf("cooldown call %d expected ErrorTypeRateLimited, got %#v", i+1, res)
		}
		c.rateLimitMu.RLock()
		after := c.rateLimits["index.docker.io"].Until
		c.rateLimitMu.RUnlock()
		if !after.Equal(before) {
			t.Fatalf("本地冷却结果修改了截止时间：%v -> %v", before, after)
		}
		if officialHits.Load() != initialHits {
			t.Fatalf("expected no additional official request during cooldown, got %d (want %d)", officialHits.Load(), initialHits)
		}
	}

	// 冷却期内 mirror 恢复时应成功返回 mirror digest，仍不访问官方
	mirrorHealthy.Store(true)
	resMirror := c.GetRemoteDigestsBatchWithPlatforms(context.Background(), q, false, false, 1)[q[0].Key()]
	if resMirror.Error != nil || resMirror.IndexDigest != mirrorDigest {
		t.Fatalf("expected mirror digest %q during cooldown, got %#v", mirrorDigest, resMirror)
	}
	if officialHits.Load() != initialHits {
		t.Fatalf("expected mirror hit without touching official, got %d (want %d)", officialHits.Load(), initialHits)
	}

	// 模拟冷却过期并让 mirror 再次失败，应重新请求官方仓库并拿到官方 digest
	mirrorHealthy.Store(false)
	officialReturn429.Store(false)
	c.rateLimitMu.Lock()
	c.rateLimits["index.docker.io"] = rateLimitState{Until: time.Now().Add(-time.Second)}
	c.rateLimitMu.Unlock()

	resRecovered := c.GetRemoteDigestsBatchWithPlatforms(context.Background(), q, false, false, 1)[q[0].Key()]
	if resRecovered.Error != nil || resRecovered.IndexDigest != officialDigest {
		t.Fatalf("expected official digest %q after cooldown expiry, got %#v", officialDigest, resRecovered)
	}
	if officialHits.Load() <= initialHits {
		t.Fatalf("expected official hits to increase after cooldown expiry, got %d (initial %d)", officialHits.Load(), initialHits)
	}
}

func padJSONManifest(base string, targetLen int) []byte {
	// base ends with `}`
	trimmed := strings.TrimSpace(base)
	extraNeeded := targetLen - len(trimmed)
	if extraNeeded < 0 {
		panic("base manifest exceeds targetLen")
	}
	// Insert spaces right before the closing `}` to keep JSON valid at exact byte length.
	return []byte(trimmed[:len(trimmed)-1] + strings.Repeat(" ", extraNeeded) + "}")
}

func TestMirrorManifestSizeLimitAndJSONValidation(t *testing.T) {
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

	const baseManifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[]}`
	exact4MiB := padJSONManifest(baseManifest, 4<<20)
	exact4MiBSum := sha256.Sum256(exact4MiB)
	exact4MiBDigest := "sha256:" + hex.EncodeToString(exact4MiBSum[:])

	over4MiB := padJSONManifest(baseManifest, (4<<20)+1)
	over4MiBSum := sha256.Sum256(over4MiB)
	over4MiBDigest := "sha256:" + hex.EncodeToString(over4MiBSum[:])

	const mirror2Manifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":2},"layers":[]}`
	m2Sum := sha256.Sum256([]byte(mirror2Manifest))
	m2Digest := "sha256:" + hex.EncodeToString(m2Sum[:])

	var m1Payload atomic.Value
	var m1DigestHdr atomic.Value
	var m1Hits, m2Hits atomic.Int32

	m1Payload.Store(over4MiB)
	m1DigestHdr.Store("")

	mirror1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m1Hits.Add(1)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		if hdr, _ := m1DigestHdr.Load().(string); hdr != "" {
			w.Header().Set("Docker-Content-Digest", hdr)
		}
		_, _ = w.Write(m1Payload.Load().([]byte))
	}))
	defer mirror1.Close()

	mirror2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m2Hits.Add(1)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", m2Digest)
		_, _ = w.Write([]byte(mirror2Manifest))
	}))
	defer mirror2.Close()

	yamlCfg := "registry:\n  mirrors:\n    - name: m1\n      url: " + mirror1.URL + "\n      enabled: true\n    - name: m2\n      url: " + mirror2.URL + "\n      enabled: true\n"
	if err := os.WriteFile(cfgPath, []byte(yamlCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(cfgPath); err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	c := New()

	// 1. > 4 MiB without Docker-Content-Digest must fall back to mirror2 even with NeedConfigDigest=false
	res := c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{
		{ImageRef: "alpine:case1", OS: "linux", Arch: "amd64", NeedConfigDigest: false},
	}, false, false, 1)["alpine:case1|linux/amd64/"]
	if res.Error != nil || res.IndexDigest != m2Digest {
		t.Fatalf("over-4MiB without header: expected fallback mirror2 digest %q, got %#v", m2Digest, res)
	}
	if m1Hits.Load() != 1 || m2Hits.Load() != 1 {
		t.Fatalf("over-4MiB without header hits mismatch: m1=%d m2=%d", m1Hits.Load(), m2Hits.Load())
	}

	// 2. > 4 MiB WITH Docker-Content-Digest must still reject truncated read and fall back to mirror2
	m1DigestHdr.Store(over4MiBDigest)
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{
		{ImageRef: "alpine:case2", OS: "linux", Arch: "amd64", NeedConfigDigest: false},
	}, false, false, 1)["alpine:case2|linux/amd64/"]
	if res.Error != nil || res.IndexDigest != m2Digest {
		t.Fatalf("over-4MiB with header: expected fallback mirror2 digest %q, got %#v", m2Digest, res)
	}
	if m1Hits.Load() != 2 || m2Hits.Load() != 2 {
		t.Fatalf("over-4MiB with header hits mismatch: m1=%d m2=%d", m1Hits.Load(), m2Hits.Load())
	}

	// 3. Exact 4 MiB valid JSON must be accepted from mirror1 without calling mirror2
	m1Payload.Store(exact4MiB)
	m1DigestHdr.Store("")
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{
		{ImageRef: "alpine:case3", OS: "linux", Arch: "amd64", NeedConfigDigest: false},
	}, false, false, 1)["alpine:case3|linux/amd64/"]
	if res.Error != nil || res.IndexDigest != exact4MiBDigest {
		t.Fatalf("exact 4MiB: expected mirror1 digest %q, got %#v", exact4MiBDigest, res)
	}
	if m1Hits.Load() != 3 || m2Hits.Load() != 2 {
		t.Fatalf("exact 4MiB hits mismatch: m1=%d m2=%d", m1Hits.Load(), m2Hits.Load())
	}

	// 4. Short corrupt JSON with NeedConfigDigest=false must fail JSON parsing and fall back to mirror2
	m1Payload.Store([]byte(`{"schemaVersion":2,`))
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{
		{ImageRef: "alpine:case4", OS: "linux", Arch: "amd64", NeedConfigDigest: false},
	}, false, false, 1)["alpine:case4|linux/amd64/"]
	if res.Error != nil || res.IndexDigest != m2Digest {
		t.Fatalf("corrupt JSON: expected fallback mirror2 digest %q, got %#v", m2Digest, res)
	}
	if m1Hits.Load() != 4 || m2Hits.Load() != 3 {
		t.Fatalf("corrupt JSON hits mismatch: m1=%d m2=%d", m1Hits.Load(), m2Hits.Load())
	}

	// 5. HTTP 200 JSON error body (valid JSON, no config.digest) with NeedConfigDigest=false must reject and fall back to mirror2
	m1Payload.Store([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"denied"}]}`))
	m1DigestHdr.Store("")
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{
		{ImageRef: "alpine:case5", OS: "linux", Arch: "amd64", NeedConfigDigest: false},
	}, false, false, 1)["alpine:case5|linux/amd64/"]
	if res.Error != nil || res.IndexDigest != m2Digest {
		t.Fatalf("HTTP 200 error JSON: expected fallback mirror2 digest %q, got %#v", m2Digest, res)
	}

	// 6. registry-1.docker.io/library/alpine:case6 hits mirrors cleanly and verifies sha512 quoted Docker-Content-Digest
	sha512Sum := sha512.Sum512([]byte(baseManifest))
	sha512Digest := "sha512:" + hex.EncodeToString(sha512Sum[:])
	m1Payload.Store([]byte(baseManifest))
	m1DigestHdr.Store(`"` + sha512Digest + `"`)
	q6 := DigestQuery{ImageRef: " registry-1.docker.io/library/alpine:case6 ", OS: "linux", Arch: "amd64", NeedConfigDigest: false}
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q6}, false, false, 1)[q6.Key()]
	if res.Error != nil || res.IndexDigest != sha512Digest {
		t.Fatalf("registry-1.docker.io + sha512 header: expected %q, got %#v", sha512Digest, res)
	}

	// 7. selectBearerWWWAuthenticate picks Bearer challenge when Basic comes first
	hdr := http.Header{}
	hdr.Add("Www-Authenticate", `Basic realm="https://bad.example.com/"`)
	hdr.Add("Www-Authenticate", `Bearer realm="https://auth.example.com/token",service="mirror",scope="repository:library/alpine:pull"`)
	picked := selectBearerWWWAuthenticate(hdr)
	realm, service, scope, err := parseWWWAuthenticate(picked)
	if err != nil || realm != "https://auth.example.com/token" || service != "mirror" || scope != "repository:library/alpine:pull" {
		t.Fatalf("unexpected Bearer challenge parse: realm=%q service=%q scope=%q err=%v", realm, service, scope, err)
	}
	if _, _, _, err := parseWWWAuthenticate(`Basic realm="https://bad.example.com/"`); err == nil {
		t.Fatalf("expected Basic Www-Authenticate scheme rejected")
	}

	// 8. RFC 7235 逗号合并 Www-Authenticate（Basic 在前、Basic 在后，以及带逗号引号 scope）
	combined1 := http.Header{}
	combined1.Add("Www-Authenticate", `Basic realm="Registry", Bearer realm="https://auth.example.com/token",service="mirror",scope="repository:library/alpine:pull,push"`)
	r1, s1, sc1, err1 := parseWWWAuthenticate(selectBearerWWWAuthenticate(combined1))
	if err1 != nil || r1 != "https://auth.example.com/token" || s1 != "mirror" || sc1 != "repository:library/alpine:pull,push" {
		t.Fatalf("combined Basic+Bearer parse failed: r=%q s=%q sc=%q err=%v", r1, s1, sc1, err1)
	}

	combined2 := http.Header{}
	combined2.Add("Www-Authenticate", `Bearer realm="https://auth.docker.io/token",service="registry.docker.io", Basic realm="Fallback"`)
	r2, s2, sc2, err2 := parseWWWAuthenticate(selectBearerWWWAuthenticate(combined2))
	if err2 != nil || r2 != "https://auth.docker.io/token" || s2 != "registry.docker.io" || sc2 != "" {
		t.Fatalf("combined Bearer+Basic parse failed (realm overwritten?): r=%q s=%q sc=%q err=%v", r2, s2, sc2, err2)
	}

	// 9. digest-referenced 镜像查询必须校检响应体哈希与请求 digest 一致，不匹配则拒绝并回退 mirror2
	m1WrongDigestBody := []byte(baseManifest)
	m1Payload.Store(m1WrongDigestBody)
	m1DigestHdr.Store("")
	pinnedRef := "alpine@" + m2Digest
	qPinned := DigestQuery{ImageRef: pinnedRef, OS: "linux", Arch: "amd64", NeedConfigDigest: false}
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{qPinned}, false, false, 1)[qPinned.Key()]
	if res.Error != nil || res.IndexDigest != m2Digest {
		t.Fatalf("digest mismatch on mirror1 should fallback to mirror2 %q, got %#v", m2Digest, res)
	}

	// 10. index manifest 缺少有效 digest 字段（零值 v1.Hash.String()==":"）时不得通过校验，应回退到 mirror2
	m1Payload.Store([]byte(`{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}}]}`))
	m1DigestHdr.Store("")
	qMissingDescDigest := DigestQuery{ImageRef: "alpine:case10", OS: "linux", Arch: "amd64", NeedConfigDigest: false}
	res = c.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{qMissingDescDigest}, false, false, 1)[qMissingDescDigest.Key()]
	if res.Error != nil || res.IndexDigest != m2Digest || res.ChildDigest == ":" {
		t.Fatalf("index manifest missing child digest should fallback to mirror2 %q, got %#v", m2Digest, res)
	}
}

type recordedRegistryReq struct {
	Scheme     string
	Host       string
	Path       string
	DialedAddr string
}

type recordingRegistryTransport struct {
	base       *http.Transport
	targetAddr string
	mu         sync.Mutex
	reqs       []recordedRegistryReq
	dialed     []string
}

func (rt *recordingRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.reqs = append(rt.reqs, recordedRegistryReq{
		Scheme: req.URL.Scheme,
		Host:   req.URL.Host,
		Path:   req.URL.Path,
	})
	rt.mu.Unlock()
	return rt.base.RoundTrip(req)
}

func (rt *recordingRegistryTransport) snapshot() ([]recordedRegistryReq, []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]recordedRegistryReq(nil), rt.reqs...), append([]string(nil), rt.dialed...)
}

func TestFetchDigestExplicitHTTPAndPortScoping(t *testing.T) {
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

	const manifestBody = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[]}`
	sum := sha256.Sum256([]byte(manifestBody))
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])

	mockReg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/v2/team/app/manifests/"):
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", wantDigest)
			_, _ = w.Write([]byte(manifestBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockReg.Close()

	newTestClient := func(t *testing.T, authYAML string) (*Client, *recordingRegistryTransport) {
		t.Helper()
		dir := t.TempDir()
		conf.EnvCfg.CONFIG_PATH = dir
		conf.EnvCfg.CONFIG_FILE = "config.yaml"
		cfgPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfgPath, []byte(authYAML), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.Load(cfgPath); err != nil {
			t.Fatalf("load config failed: %v", err)
		}

		rt := &recordingRegistryTransport{targetAddr: mockReg.Listener.Addr().String()}
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		rt.base = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				rt.mu.Lock()
				rt.dialed = append(rt.dialed, addr)
				rt.mu.Unlock()
				return dialer.DialContext(ctx, "tcp", rt.targetAddr)
			},
		}

		c := New()
		c.roundTripper = rt
		return c, rt
	}

	// 1. 显式 HTTP 域名无端口按 tag 查询：首次即走 HTTP 默认 80 端口并返回摘要
	c1, rt1 := newTestClient(t, "registry:\n  auth:\n    - host: http://registry.example.com\n      username: u\n      token: p\n")
	q1 := DigestQuery{ImageRef: "registry.example.com/team/app:v1", OS: "linux", Arch: "amd64"}
	res1 := c1.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q1}, false, false, 1)[q1.Key()]
	if res1.Error != nil || res1.IndexDigest != wantDigest {
		t.Fatalf("explicit http without port failed: %#v", res1)
	}
	reqs1, dialed1 := rt1.snapshot()
	if len(reqs1) == 0 || reqs1[0].Scheme != "http" {
		t.Fatalf("expected first request over http, got %#v", reqs1)
	}
	if len(dialed1) == 0 || dialed1[0] != "registry.example.com:80" {
		t.Fatalf("expected dialed addr registry.example.com:80, got %#v", dialed1)
	}

	// 2. 显式 HTTP 域名带 :5000 按 digest 查询：首次即走 HTTP 发往 :5000
	c2, rt2 := newTestClient(t, "registry:\n  auth:\n    - host: http://registry.example.com:5000\n")
	q2 := DigestQuery{ImageRef: "registry.example.com:5000/team/app@" + wantDigest, OS: "linux", Arch: "amd64"}
	res2 := c2.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q2}, false, false, 1)[q2.Key()]
	if res2.Error != nil || res2.IndexDigest != wantDigest {
		t.Fatalf("explicit http :5000 by digest failed: %#v", res2)
	}
	reqs2, dialed2 := rt2.snapshot()
	if len(reqs2) == 0 || reqs2[0].Scheme != "http" {
		t.Fatalf("expected first request over http, got %#v", reqs2)
	}
	if len(dialed2) == 0 || dialed2[0] != "registry.example.com:5000" {
		t.Fatalf("expected dialed addr registry.example.com:5000, got %#v", dialed2)
	}

	// 3. 同名域名 HTTP 配置只匹配 :5000 时，查询其他端口或默认端口仍走默认 HTTPS
	c3, rt3 := newTestClient(t, "registry:\n  auth:\n    - host: http://registry.example.com:5000\n")
	for _, ref := range []string{"registry.example.com:5443/team/app:v1", "registry.example.com/team/app:v1"} {
		q := DigestQuery{ImageRef: ref, OS: "linux", Arch: "amd64"}
		_ = c3.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q}, false, false, 1)
	}
	reqs3, _ := rt3.snapshot()
	if len(reqs3) < 2 {
		t.Fatalf("expected >= 2 requests, got %#v", reqs3)
	}
	for _, req := range reqs3 {
		if req.Scheme != "https" {
			t.Fatalf("expected unmatched port on same domain to use https, got %#v", reqs3)
		}
	}

	// 4. 普通公网域名无 HTTP 配置时 TLS 失败，且绝不降级发 HTTP 请求
	c4, rt4 := newTestClient(t, "logging:\n  level: info\n")
	q4 := DigestQuery{ImageRef: "registry.example.com/team/app:v1", OS: "linux", Arch: "amd64"}
	res4 := c4.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q4}, false, false, 1)[q4.Key()]
	if res4.Error == nil {
		t.Fatalf("expected TLS error for plain-HTTP mock without explicit HTTP config, got %#v", res4)
	}
	reqs4, _ := rt4.snapshot()
	for _, req := range reqs4 {
		if req.Scheme == "http" {
			t.Fatalf("expected zero http requests for unconfigured public domain, got %#v", reqs4)
		}
	}
}

func TestIndexManifestARMVariantMatchingAndMirrorFallback(t *testing.T) {
	original := conf.EnvCfg
	testEnv := *original
	conf.EnvCfg = &testEnv
	t.Cleanup(func() {
		conf.EnvCfg = original
	})
	origCfg := config.Get()
	t.Cleanup(func() {
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
	})

	v6ConfigBody := `{"architecture":"arm","variant":"v6","os":"linux","rootfs":{"type":"layers","diff_ids":[]}}`
	v6CfgSum := sha256.Sum256([]byte(v6ConfigBody))
	v6CfgDigest := "sha256:" + hex.EncodeToString(v6CfgSum[:])
	v6Manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + v6CfgDigest + `","size":` + fmt.Sprintf("%d", len(v6ConfigBody)) + `},"layers":[]}`
	v6Sum := sha256.Sum256([]byte(v6Manifest))
	v6Digest := "sha256:" + hex.EncodeToString(v6Sum[:])

	v7ConfigBody := `{"architecture":"arm","variant":"v7","os":"linux","rootfs":{"type":"layers","diff_ids":[]}}`
	v7CfgSum := sha256.Sum256([]byte(v7ConfigBody))
	v7CfgDigest := "sha256:" + hex.EncodeToString(v7CfgSum[:])
	v7Manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + v7CfgDigest + `","size":` + fmt.Sprintf("%d", len(v7ConfigBody)) + `},"layers":[]}`
	v7Sum := sha256.Sum256([]byte(v7Manifest))
	v7Digest := "sha256:" + hex.EncodeToString(v7Sum[:])

	// v7 放在首位，确保匹配逻辑不会因为顺序优先而误选 v7
	v7OnlyIndex := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + v7Digest + `","size":123,"platform":{"os":"linux","architecture":"arm","variant":"v7"}}]}`
	v7OnlyIndexSum := sha256.Sum256([]byte(v7OnlyIndex))
	v7OnlyIndexDigest := "sha256:" + hex.EncodeToString(v7OnlyIndexSum[:])

	bothIndex := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + v7Digest + `","size":123,"platform":{"os":"linux","architecture":"arm","variant":"v7"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + v6Digest + `","size":123,"platform":{"os":"linux","architecture":"arm","variant":"v6"}}]}`
	bothIndexSum := sha256.Sum256([]byte(bothIndex))
	bothIndexDigest := "sha256:" + hex.EncodeToString(bothIndexSum[:])

	v6OnlyIndex := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + v6Digest + `","size":123,"platform":{"os":"linux","architecture":"arm","variant":"v6"}}]}`
	v6OnlyIndexSum := sha256.Sum256([]byte(v6OnlyIndex))
	v6OnlyIndexDigest := "sha256:" + hex.EncodeToString(v6OnlyIndexSum[:])

	blobsByDigest := map[string]string{
		v6CfgDigest: v6ConfigBody,
		v7CfgDigest: v7ConfigBody,
	}

	serveRegistry := func(indexBody, indexDigest string, manifestsByDigest map[string]string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
				w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
				w.WriteHeader(http.StatusOK)
				return
			}
			const blobPrefix = "/v2/library/armapp/blobs/"
			if strings.HasPrefix(r.URL.Path, blobPrefix) {
				ref := strings.TrimPrefix(r.URL.Path, blobPrefix)
				if blob, ok := blobsByDigest[ref]; ok {
					w.Header().Set("Content-Type", "application/vnd.oci.image.config.v1+json")
					w.Header().Set("Docker-Content-Digest", ref)
					_, _ = w.Write([]byte(blob))
					return
				}
			}
			const prefix = "/v2/library/armapp/manifests/"
			if !strings.HasPrefix(r.URL.Path, prefix) {
				http.NotFound(w, r)
				return
			}
			ref := strings.TrimPrefix(r.URL.Path, prefix)
			if childBody, ok := manifestsByDigest[ref]; ok {
				w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
				w.Header().Set("Docker-Content-Digest", ref)
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				_, _ = w.Write([]byte(childBody))
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.Header().Set("Docker-Content-Digest", indexDigest)
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write([]byte(indexBody))
		}
	}

	var m1Payload, m1Digest, m2Payload, m2Digest, upPayload, upDigest atomic.Value
	childMap := map[string]string{v6Digest: v6Manifest, v7Digest: v7Manifest}

	mirror1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := m1Payload.Load().(string)
		d, _ := m1Digest.Load().(string)
		serveRegistry(p, d, childMap)(w, r)
	}))
	defer mirror1.Close()

	mirror2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := m2Payload.Load().(string)
		d, _ := m2Digest.Load().(string)
		serveRegistry(p, d, childMap)(w, r)
	}))
	defer mirror2.Close()

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := upPayload.Load().(string)
		d, _ := upDigest.Load().(string)
		serveRegistry(p, d, childMap)(w, r)
	}))
	defer upstream.Close()

	setupClient := func(t *testing.T, withMirrors bool) *Client {
		t.Helper()
		dir := t.TempDir()
		conf.EnvCfg.CONFIG_PATH = dir
		conf.EnvCfg.CONFIG_FILE = "config.yaml"
		cfgPath := filepath.Join(dir, "config.yaml")
		yamlCfg := "logging:\n  level: info\n"
		if withMirrors {
			yamlCfg = "registry:\n  mirrors:\n    - name: m1\n      url: " + mirror1.URL + "\n      enabled: true\n    - name: m2\n      url: " + mirror2.URL + "\n      enabled: true\n"
		}
		if err := os.WriteFile(cfgPath, []byte(yamlCfg), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.Load(cfgPath); err != nil {
			t.Fatalf("load config failed: %v", err)
		}

		upstreamAddr := upstream.Listener.Addr().String()
		tr := upstream.Client().Transport.(*http.Transport).Clone()
		if tr.TLSClientConfig != nil {
			tr.TLSClientConfig = tr.TLSClientConfig.Clone()
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err == nil && (host == "index.docker.io" || host == "registry-1.docker.io") {
				return dialer.DialContext(ctx, network, upstreamAddr)
			}
			return dialer.DialContext(ctx, network, addr)
		}
		c := New()
		c.httpTransport = tr
		return c
	}

	// 1. 原站仅有 linux/arm/v7，查询 linux/arm/v6（NeedConfigDigest=false 与 true 均须返回失败，包含平台信息）
	upPayload.Store(v7OnlyIndex)
	upDigest.Store(v7OnlyIndexDigest)
	cNoMirrors := setupClient(t, false)
	for _, needCfg := range []bool{false, true} {
		tag := "armapp:v7only-false"
		if needCfg {
			tag = "armapp:v7only-true"
		}
		q := DigestQuery{ImageRef: tag, OS: "linux", Arch: "arm", Variant: "v6", NeedConfigDigest: needCfg}
		res := cNoMirrors.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q}, false, false, 1)[q.Key()]
		if res.Error == nil {
			t.Fatalf("needConfig=%v: expected missing linux/arm/v6 error, got %#v", needCfg, res)
		}
		if !strings.Contains(res.Error.Error(), "linux/arm/v6") {
			t.Fatalf("needConfig=%v: expected error mentioning linux/arm/v6, got %q", needCfg, res.Error.Error())
		}
		if res.ChildDigest != "" {
			t.Fatalf("needConfig=%v: expected empty ChildDigest on missing variant, got %#v", needCfg, res)
		}
	}

	// 2. 索引同时包含 v7 与 v6 时，按请求的变体精确返回
	upPayload.Store(bothIndex)
	upDigest.Store(bothIndexDigest)
	qV6 := DigestQuery{ImageRef: "armapp:both", OS: "linux", Arch: "arm", Variant: "v6", NeedConfigDigest: true}
	qV7 := DigestQuery{ImageRef: "armapp:both", OS: "linux", Arch: "arm", Variant: "v7", NeedConfigDigest: false}
	batch := cNoMirrors.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{qV6, qV7}, false, false, 2)
	if got := batch[qV6.Key()]; got.Error != nil || got.IndexDigest != bothIndexDigest || got.ChildDigest != v6Digest || got.ConfigDigest != v6CfgDigest {
		t.Fatalf("v6 exact match mismatch: %#v (err=%v)", got, got.Error)
	}
	if got := batch[qV7.Key()]; got.Error != nil || got.IndexDigest != bothIndexDigest || got.ChildDigest != v7Digest {
		t.Fatalf("v7 exact match mismatch: %#v", got)
	}

	// 3. mirror1 仅含 v7、mirror2 含 v6 时，跳过 mirror1 并返回 mirror2 的 v6 子摘要（覆盖 NeedConfigDigest=false 与 true）
	m1Payload.Store(v7OnlyIndex)
	m1Digest.Store(v7OnlyIndexDigest)
	m2Payload.Store(v6OnlyIndex)
	m2Digest.Store(v6OnlyIndexDigest)
	upPayload.Store(v7OnlyIndex)
	upDigest.Store(v7OnlyIndexDigest)
	cWithMirrors := setupClient(t, true)
	for _, needCfg := range []bool{false, true} {
		tag := "armapp:mirror-false"
		if needCfg {
			tag = "armapp:mirror-true"
		}
		q := DigestQuery{ImageRef: tag, OS: "linux", Arch: "arm", Variant: "v6", NeedConfigDigest: needCfg}
		res := cWithMirrors.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q}, false, false, 1)[q.Key()]
		if res.Error != nil || res.IndexDigest != v6OnlyIndexDigest || res.ChildDigest != v6Digest {
			t.Fatalf("needConfig=%v: expected mirror2 v6 digest, got %#v", needCfg, res)
		}
		if needCfg && res.ConfigDigest != v6CfgDigest {
			t.Fatalf("needConfig=true: expected v6 config digest %q, got %#v", v6CfgDigest, res)
		}
	}

	// 4. 所有 mirror 和原站均缺少 v6 时，无论 NeedConfigDigest 为何值均报错
	m2Payload.Store(v7OnlyIndex)
	m2Digest.Store(v7OnlyIndexDigest)
	for _, needCfg := range []bool{false, true} {
		tag := "armapp:allmiss-false"
		if needCfg {
			tag = "armapp:allmiss-true"
		}
		q := DigestQuery{ImageRef: tag, OS: "linux", Arch: "arm", Variant: "v6", NeedConfigDigest: needCfg}
		res := cWithMirrors.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{q}, false, false, 1)[q.Key()]
		if res.Error == nil || !strings.Contains(res.Error.Error(), "linux/arm/v6") {
			t.Fatalf("needConfig=%v: expected linux/arm/v6 error when all mirrors + upstream lack v6, got %#v", needCfg, res)
		}
	}

	// 5. 省略变体（Variant=""）仍能按 OS + Arch 回退匹配 v7
	qOmitVar := DigestQuery{ImageRef: "armapp:omitvar", OS: "linux", Arch: "arm", Variant: "", NeedConfigDigest: false}
	resOmit := cWithMirrors.GetRemoteDigestsBatchWithPlatforms(context.Background(), []DigestQuery{qOmitVar}, false, false, 1)[qOmitVar.Key()]
	if resOmit.Error != nil || resOmit.IndexDigest != v7OnlyIndexDigest || resOmit.ChildDigest != v7Digest {
		t.Fatalf("omitted variant fallback mismatch: %#v", resOmit)
	}
}
