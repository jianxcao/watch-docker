package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"github.com/jianxcao/watch-docker/backend/internal/registry"
)

func TestExtractAndCompareDigestsAllAlgorithms(t *testing.T) {
	sha256Hex := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha384Hex := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sha512Hex := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	if got := extractDigest("sha256:" + sha256Hex); got != "sha256:"+sha256Hex {
		t.Fatalf("extract sha256 bare = %q", got)
	}
	if got := extractDigest("sha384:" + sha384Hex); got != "sha384:"+sha384Hex {
		t.Fatalf("extract sha384 bare = %q", got)
	}
	if got := extractDigest("sha512:" + sha512Hex); got != "sha512:"+sha512Hex {
		t.Fatalf("extract sha512 bare = %q", got)
	}
	if got := extractDigest("docker.io/library/alpine@sha512:" + sha512Hex); got != "sha512:"+sha512Hex {
		t.Fatalf("extract repo@sha512 = %q", got)
	}

	if !compareDigests([]string{"sha384:" + sha384Hex}, "SHA384:"+sha384Hex) {
		t.Fatal("expected bare sha384 digest to match case-insensitively")
	}
	if !compareDigests([]string{"sha512:" + sha512Hex}, "sha512:"+sha512Hex) {
		t.Fatal("expected bare sha512 digest to match")
	}
	if compareDigests([]string{"sha512:" + sha512Hex}, "sha512:0000") {
		t.Fatal("expected mismatched sha512 digest not to match")
	}
}

func TestScanOnceUsesPerContainerPlatformForMultiArchAndArm64OnlyImages(t *testing.T) {
	ctx := context.Background()

	origEnv := conf.EnvCfg
	origCfg := config.Get()
	dir := t.TempDir()
	testEnv := *origEnv
	testEnv.CONFIG_PATH = dir
	testEnv.CONFIG_FILE = "config.yaml"
	conf.EnvCfg = &testEnv
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("scan:\n  allowComposeUpdate: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(cfgPath); err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	t.Cleanup(func() {
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
		conf.EnvCfg = origEnv
	})

	const amd64Cfg = `{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":[]}}`
	amd64CfgSum := sha256.Sum256([]byte(amd64Cfg))
	amd64CfgDigest := "sha256:" + hex.EncodeToString(amd64CfgSum[:])

	const arm64Cfg = `{"architecture":"arm64","os":"linux","rootfs":{"type":"layers","diff_ids":[]}}`
	arm64CfgSum := sha256.Sum256([]byte(arm64Cfg))
	arm64CfgDigest := "sha256:" + hex.EncodeToString(arm64CfgSum[:])

	amd64Manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + amd64CfgDigest + `","size":65},"layers":[]}`
	amd64Sum := sha256.Sum256([]byte(amd64Manifest))
	amd64Digest := "sha256:" + hex.EncodeToString(amd64Sum[:])

	arm64Manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + arm64CfgDigest + `","size":65},"layers":[]}`
	arm64Sum := sha256.Sum256([]byte(arm64Manifest))
	arm64Digest := "sha256:" + hex.EncodeToString(arm64Sum[:])

	multiArchIndex := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[` +
		`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + amd64Digest + `","size":` + jsonInt(len(amd64Manifest)) + `,"platform":{"os":"linux","architecture":"amd64"}},` +
		`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + arm64Digest + `","size":` + jsonInt(len(arm64Manifest)) + `,"platform":{"os":"linux","architecture":"arm64","variant":"v8"}}` +
		`]}`
	multiIndexSum := sha256.Sum256([]byte(multiArchIndex))
	multiIndexDigest := "sha256:" + hex.EncodeToString(multiIndexSum[:])

	arm64OnlyIndex := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[` +
		`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + arm64Digest + `","size":` + jsonInt(len(arm64Manifest)) + `,"platform":{"os":"linux","architecture":"arm64","variant":"v8"}}` +
		`]}`
	arm64OnlySum := sha256.Sum256([]byte(arm64OnlyIndex))
	arm64OnlyDigest := "sha256:" + hex.EncodeToString(arm64OnlySum[:])

	var mu sync.Mutex
	var requestedPaths []string
	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/v2/", "/v2":
			w.WriteHeader(http.StatusOK)
		case "/v2/demo/multi/manifests/latest":
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.Header().Set("Docker-Content-Digest", multiIndexDigest)
			_, _ = w.Write([]byte(multiArchIndex))
		case "/v2/demo/armonly/manifests/latest":
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.Header().Set("Docker-Content-Digest", arm64OnlyDigest)
			_, _ = w.Write([]byte(arm64OnlyIndex))
		case "/v2/demo/multi/manifests/" + amd64Digest, "/v2/demo/armonly/manifests/" + amd64Digest:
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", amd64Digest)
			_, _ = w.Write([]byte(amd64Manifest))
		case "/v2/demo/multi/manifests/" + arm64Digest, "/v2/demo/armonly/manifests/" + arm64Digest:
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", arm64Digest)
			_, _ = w.Write([]byte(arm64Manifest))
		case "/v2/demo/multi/blobs/" + amd64CfgDigest, "/v2/demo/armonly/blobs/" + amd64CfgDigest:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(amd64Cfg))
		case "/v2/demo/multi/blobs/" + arm64CfgDigest, "/v2/demo/armonly/blobs/" + arm64CfgDigest:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(arm64Cfg))
		default:
			http.NotFound(w, r)
		}
	}))
	defer regSrv.Close()

	regHost := regSrv.Listener.Addr().String()
	multiRef := regHost + "/demo/multi:latest"
	armOnlyRef := regHost + "/demo/armonly:latest"

	dockerContainers := []map[string]any{
		{
			"Id":      "c-amd64",
			"Names":   []string{"/c-amd64"},
			"Image":   multiRef,
			"ImageID": "sha256:shared-img",
			"State":   "running",
			"ImageManifestDescriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    amd64Digest,
				"size":      len(amd64Manifest),
				"platform":  map[string]any{"os": "linux", "architecture": "amd64"},
			},
		},
		{
			"Id":      "c-arm64",
			"Names":   []string{"/c-arm64"},
			"Image":   multiRef,
			"ImageID": "sha256:shared-img",
			"State":   "running",
			"ImageManifestDescriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    arm64Digest,
				"size":      len(arm64Manifest),
				"platform":  map[string]any{"os": "linux", "architecture": "arm64", "variant": "v8"},
			},
		},
		{
			"Id":      "c-armonly",
			"Names":   []string{"/c-armonly"},
			"Image":   armOnlyRef,
			"ImageID": "sha256:armonly-img",
			"State":   "running",
			"ImageManifestDescriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    arm64Digest,
				"size":      len(arm64Manifest),
				"platform":  map[string]any{"os": "linux", "architecture": "arm64", "variant": "v8"},
			},
		},
	}

	inspects := map[string]map[string]any{
		// 刻意把 ImageInspect 标为 linux/amd64、RepoDigests 只放 amd64Digest，模拟同镜像下 inspect 掩盖 arm64 容器实际平台
		"sha256:shared-img": {
			"Id":           "sha256:shared-img",
			"Os":           "linux",
			"Architecture": "amd64",
			"RepoTags":     []string{multiRef},
			"RepoDigests":  []string{regHost + "/demo/multi@" + amd64Digest},
		},
		"sha256:armonly-img": {
			"Id":           "sha256:armonly-img",
			"Os":           "linux",
			"Architecture": "amd64",
			"RepoTags":     []string{armOnlyRef},
			"RepoDigests":  []string{regHost + "/demo/armonly@" + arm64Digest},
		},
	}

	dockerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dockerContainers)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Id":    "c",
				"State": map[string]any{"StartedAt": "2026-11-26T00:00:00Z"},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			trimmed := strings.TrimSuffix(r.URL.Path, "/json")
			imgID := trimmed[strings.LastIndex(trimmed, "/")+1:]
			resp, ok := inspects[imgID]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer dockerSrv.Close()

	dcli, err := dockercli.New(ctx, "tcp://"+dockerSrv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("create dockercli: %v", err)
	}
	rcli := registry.New()
	sc := New(dcli, rcli)

	statuses, err := sc.ScanOnce(ctx, true, 2, false, true)
	if err != nil {
		t.Fatalf("ScanOnce failed: %v", err)
	}
	byName := make(map[string]ContainerStatus, len(statuses))
	for _, st := range statuses {
		byName[st.Name] = st
	}

	if got := byName["c-armonly"]; got.Status != "UpToDate" {
		t.Fatalf("expected c-armonly to resolve arm64 platform and report UpToDate, got: %#v", got)
	}
	if got := byName["c-amd64"]; got.Status != "UpToDate" {
		t.Fatalf("expected c-amd64 UpToDate, got: %#v", got)
	}
	if got := byName["c-arm64"]; got.Status != "UpdateAvailable" {
		t.Fatalf("expected c-arm64 to resolve arm64 digest (distinct from cached amd64 digest), got: %#v", got)
	}

	mu.Lock()
	pathsCopy := append([]string(nil), requestedPaths...)
	mu.Unlock()
	multiLatestHits := 0
	armOnlyHits := 0
	for _, p := range pathsCopy {
		switch p {
		case "GET /v2/demo/multi/manifests/latest":
			multiLatestHits++
		case "GET /v2/demo/armonly/manifests/latest":
			armOnlyHits++
		}
	}
	if multiLatestHits != 2 {
		t.Fatalf("expected 2 distinct platform registry queries for shared multi-arch image, got %d (paths=%v)", multiLatestHits, pathsCopy)
	}
	if armOnlyHits != 1 {
		t.Fatalf("expected 1 registry query for armonly image, got %d (paths=%v)", armOnlyHits, pathsCopy)
	}
}

func jsonInt(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
