package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	conf.EnvCfg.CONFIG_PATH = dir
	conf.EnvCfg.CONFIG_FILE = "config.yaml"
	cfgPath := filepath.Join(dir, "config.yaml")
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
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
	conf.EnvCfg.CONFIG_PATH = dir
	conf.EnvCfg.CONFIG_FILE = "config.yaml"
	cfgPath := filepath.Join(dir, "config.yaml")
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
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
}
