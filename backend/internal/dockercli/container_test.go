package dockercli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

func TestComparableRepoDigestsUsesManifestFallback(t *testing.T) {
	img := image.InspectResponse{
		ID: "sha256:config-digest",
		Manifests: []image.ManifestSummary{
			{ID: "sha256:manifest-a"},
			{ID: "sha256:manifest-b"},
		},
	}

	got := comparableRepoDigests(img)

	want := []string{"sha256:manifest-a", "sha256:manifest-b"}
	if len(got) != len(want) {
		t.Fatalf("expected %d digests, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected digest %q at index %d, got %q", want[i], i, got[i])
		}
	}
}

func TestComparableRepoDigestsDoesNotUseImageIDFallback(t *testing.T) {
	img := image.InspectResponse{
		ID: "sha256:config-digest",
	}

	got := comparableRepoDigests(img)

	if len(got) != 0 {
		t.Fatalf("expected no comparable digests, got %#v", got)
	}
}

func TestIsAnonymousVolume(t *testing.T) {
	hex64 := strings.Repeat("a1", 32)
	if !isAnonymousVolume(hex64, nil) {
		t.Fatalf("expected 64-char hex volume without compose labels to be anonymous")
	}
	if isAnonymousVolume("wd_e2e_long_named_volume_should_never_be_deleted_999", nil) {
		t.Fatalf("expected long named volume with underscores not to be treated as anonymous")
	}
	if isAnonymousVolume(hex64, map[string]string{"com.docker.compose.volume": "data"}) {
		t.Fatalf("expected compose named volume not to be treated as anonymous")
	}
	if !isAnonymousVolume("custom", map[string]string{"com.docker.volume.anonymous": "true"}) {
		t.Fatalf("expected explicitly labeled anonymous volume to be true")
	}
}

func TestCleanContainerPathRejectsRootDangerous(t *testing.T) {
	if p, err := cleanContainerPath("/;"); err != nil || p != "/;" {
		t.Fatalf("expected literal '/;' to be preserved without stripping to '/', got %q, err=%v", p, err)
	}
	if _, err := cleanContainerPath(""); err == nil {
		t.Fatalf("expected error for empty path")
	}
	if _, err := cleanContainerPath("/tmp\x00evil"); err == nil {
		t.Fatalf("expected error for null byte")
	}
}

type mockStatsDockerClient struct{}

func (m *mockStatsDockerClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{
		Items: []container.Summary{{ID: "1234567890abcdef1234567890abcdef"}},
	}, nil
}

func (m *mockStatsDockerClient) ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	body := `{"read":"2026-01-01T00:00:01Z","cpu_stats":{"cpu_usage":{"total_usage":200},"system_cpu_usage":1000},"memory_stats":{"usage":1024,"limit":4096}}`
	return client.ContainerStatsResult{
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestStatsManagerNoDeadlockAndConcurrentSafe(t *testing.T) {
	sm := NewStatsManager(&mockStatsDockerClient{})
	sm.statsCache["1234567890abcdef1234567890abcdef"] = &ContainerStats{
		ID:   "1234567890abcdef1234567890abcdef",
		Name: "1234567890ab",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				sm.collectSingleContainerStats(ctx, "1234567890abcdef1234567890abcdef")
				_, err := sm.GetContainersStats(ctx, []string{"1234567890abcdef1234567890abcdef", "short"})
				if err != nil {
					t.Errorf("GetContainersStats err: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestListContainersLazyParentIDLookupAndCache(t *testing.T) {
	ctx := context.Background()

	type mockState struct {
		mu             sync.Mutex
		containers     []map[string]any
		inspects       map[string]map[string]any
		imagesList     []map[string]any
		imageListErr   bool
		imageListHits  atomic.Int32
		blockImageList chan struct{}
		inImageList    chan struct{}
	}

	st := &mockState{
		inspects: make(map[string]map[string]any),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/json"):
			st.mu.Lock()
			items := st.containers
			st.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Id":    "c1",
				"State": map[string]any{"StartedAt": "2026-11-25T00:00:00Z"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/images/json" || strings.HasSuffix(r.URL.Path, "/images/json"):
			st.imageListHits.Add(1)
			st.mu.Lock()
			inCh := st.inImageList
			blockCh := st.blockImageList
			st.inImageList = nil
			st.blockImageList = nil
			errMode := st.imageListErr
			items := st.imagesList
			st.mu.Unlock()
			if inCh != nil {
				close(inCh)
			}
			if blockCh != nil {
				<-blockCh
			}
			if errMode {
				http.Error(w, "daemon busy", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			trimmed := strings.TrimSuffix(r.URL.Path, "/json")
			imgID := trimmed[strings.LastIndex(trimmed, "/")+1:]
			st.mu.Lock()
			resp, ok := st.inspects[imgID]
			st.mu.Unlock()
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
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()

	cli := &Client{
		docker:       dockerCli,
		imageCache:   make(map[string]cachedImageMeta),
		startedCache: make(map[string]cachedStartMeta),
	}

	// 1. 无容器 -> 0 次 ImageList
	st.mu.Lock()
	st.containers = []map[string]any{}
	st.mu.Unlock()
	list, err := cli.ListContainers(ctx, true)
	if err != nil || len(list) != 0 || st.imageListHits.Load() != 0 {
		t.Fatalf("empty containers: list=%#v err=%v hits=%d", list, err, st.imageListHits.Load())
	}

	// 2. 无具名标签(<none>:<none>)、已有 RepoDigests、Descriptor 摘要、显式 Build Identity -> 均 0 次 ImageList
	st.mu.Lock()
	st.containers = []map[string]any{
		{"Id": "c-dangling", "Names": []string{"/c-dangling"}, "Image": "sha256:img_dangling", "ImageID": "img_dangling", "State": "running"},
		{"Id": "c-digest", "Names": []string{"/c-digest"}, "Image": "nginx:1", "ImageID": "img_digest", "State": "running"},
		{"Id": "c-desc", "Names": []string{"/c-desc"}, "Image": "alpine:3", "ImageID": "img_desc", "State": "running"},
		{"Id": "c-build", "Names": []string{"/c-build"}, "Image": "mybuild:v1", "ImageID": "img_build", "State": "running"},
	}
	st.inspects["img_dangling"] = map[string]any{"Id": "img_dangling", "RepoTags": []string{"<none>:<none>"}}
	st.inspects["img_digest"] = map[string]any{"Id": "img_digest", "RepoTags": []string{"nginx:1"}, "RepoDigests": []string{"nginx@sha256:aaa"}}
	st.inspects["img_desc"] = map[string]any{"Id": "img_desc", "RepoTags": []string{"alpine:3"}, "Descriptor": map[string]any{"digest": "sha256:bbb"}}
	st.inspects["img_build"] = map[string]any{"Id": "img_build", "RepoTags": []string{"mybuild:v1"}, "Identity": map[string]any{"Build": []map[string]any{{"Ref": "ref1"}}}}
	st.mu.Unlock()

	list, err = cli.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers scenario 2 failed: %v", err)
	}
	if st.imageListHits.Load() != 0 {
		t.Fatalf("expected 0 ImageList calls when metadata already sufficient, got %d", st.imageListHits.Load())
	}
	localByID := make(map[string]bool, len(list))
	for _, c := range list {
		localByID[c.ID] = c.IsLocalImage
	}
	if !localByID["c-dangling"] || localByID["c-digest"] || localByID["c-desc"] || !localByID["c-build"] {
		t.Fatalf("unexpected local classifications in scenario 2: %#v", localByID)
	}

	// 3. 多个待判定 ID（含共享同一 ID 的容器、有 ParentID 和空 ParentID）-> 冷轮 1 次，热轮 0 次
	cli.InvalidateImageCache()
	st.imageListHits.Store(0)
	st.mu.Lock()
	st.containers = []map[string]any{
		{"Id": "c-local-1", "Names": []string{"/c-local-1"}, "Image": "app:dev", "ImageID": "img_need_local", "State": "running"},
		{"Id": "c-local-2", "Names": []string{"/c-local-2"}, "Image": "app:dev", "ImageID": "img_need_local", "State": "running"},
		{"Id": "c-remote-empty-parent", "Names": []string{"/c-remote"}, "Image": "pulled:v1", "ImageID": "img_need_empty_parent", "State": "running"},
	}
	st.inspects["img_need_local"] = map[string]any{"Id": "img_need_local", "RepoTags": []string{"app:dev"}}
	st.inspects["img_need_empty_parent"] = map[string]any{"Id": "img_need_empty_parent", "RepoTags": []string{"pulled:v1"}}
	st.imagesList = []map[string]any{
		{"Id": "img_need_local", "ParentID": "sha256:parent123"},
		{"Id": "img_need_empty_parent", "ParentID": ""},
	}
	st.mu.Unlock()

	coldList, err := cli.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("cold ListContainers failed: %v", err)
	}
	if got := st.imageListHits.Load(); got != 1 {
		t.Fatalf("cold round expected 1 ImageList call, got %d", got)
	}
	t.Logf("cold ImageList hits=%d", st.imageListHits.Load())

	hotList, err := cli.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("hot ListContainers failed: %v", err)
	}
	if got := st.imageListHits.Load(); got != 1 {
		t.Fatalf("hot round expected 0 extra ImageList calls (total 1), got %d", got)
	}
	t.Logf("hot ImageList total hits=%d (0 new calls)", st.imageListHits.Load())

	for _, resList := range [][]ContainerInfo{coldList, hotList} {
		m := make(map[string]bool)
		for _, c := range resList {
			m[c.ID] = c.IsLocalImage
		}
		if !m["c-local-1"] || !m["c-local-2"] || m["c-remote-empty-parent"] {
			t.Fatalf("unexpected local flags across cold/hot calls: %#v", m)
		}
	}

	// 4. InvalidateImageCache 与过期 TTL 后各重新触发 1 次 ImageList
	cli.InvalidateImageCache()
	_, err = cli.ListContainers(ctx, true)
	if err != nil || st.imageListHits.Load() != 2 {
		t.Fatalf("after InvalidateImageCache expected hits=2, got %d err=%v", st.imageListHits.Load(), err)
	}

	cli.cacheMu.Lock()
	for id, meta := range cli.imageCache {
		meta.FetchedAt = time.Now().Add(-6 * time.Minute)
		cli.imageCache[id] = meta
	}
	cli.cacheMu.Unlock()
	_, err = cli.ListContainers(ctx, true)
	if err != nil || st.imageListHits.Load() != 3 {
		t.Fatalf("after TTL expiry expected hits=3, got %d err=%v", st.imageListHits.Load(), err)
	}

	// 5. ImageList 失败与遗漏 ID 时保持 ParentKnown=false，下次完整响应时可正确更新并缓存
	cli.InvalidateImageCache()
	st.imageListHits.Store(0)
	st.mu.Lock()
	st.containers = []map[string]any{
		{"Id": "c-miss", "Names": []string{"/c-miss"}, "Image": "retry:v1", "ImageID": "img_retry", "State": "running"},
	}
	st.inspects["img_retry"] = map[string]any{"Id": "img_retry", "RepoTags": []string{"retry:v1"}}
	st.imageListErr = true
	st.mu.Unlock()

	errRound, err := cli.ListContainers(ctx, true)
	if err != nil || errRound[0].IsLocalImage || st.imageListHits.Load() != 1 {
		t.Fatalf("on ImageList failure expected fallback false and 1 call, got %#v err=%v hits=%d", errRound, err, st.imageListHits.Load())
	}

	st.mu.Lock()
	st.imageListErr = false
	st.imagesList = []map[string]any{} // 遗漏 img_retry
	st.mu.Unlock()
	missRound, err := cli.ListContainers(ctx, true)
	if err != nil || missRound[0].IsLocalImage || st.imageListHits.Load() != 2 {
		t.Fatalf("on omitted ID expected fallback false and hits=2, got %#v err=%v hits=%d", missRound, err, st.imageListHits.Load())
	}

	st.mu.Lock()
	st.imagesList = []map[string]any{{"Id": "img_retry", "ParentID": "sha256:recovered-parent"}}
	st.mu.Unlock()
	recRound, err := cli.ListContainers(ctx, true)
	if err != nil || !recRound[0].IsLocalImage || st.imageListHits.Load() != 3 {
		t.Fatalf("on recovered ID expected IsLocalImage=true and hits=3, got %#v err=%v hits=%d", recRound, err, st.imageListHits.Load())
	}
	cachedRound, err := cli.ListContainers(ctx, true)
	if err != nil || !cachedRound[0].IsLocalImage || st.imageListHits.Load() != 3 {
		t.Fatalf("on cached recovered ID expected hits still 3, got %#v err=%v hits=%d", cachedRound, err, st.imageListHits.Load())
	}

	// 6. 并发竞态保护：旧 ImageList 返回途中缓存被清并刷新出新 FetchedAt 与 ParentID，迟到响应不得覆写新条目
	cli.InvalidateImageCache()
	inCh := make(chan struct{})
	blockCh := make(chan struct{})
	st.mu.Lock()
	st.containers = []map[string]any{
		{"Id": "c-race", "Names": []string{"/c-race"}, "Image": "race:v1", "ImageID": "img_race", "State": "running"},
	}
	st.inspects["img_race"] = map[string]any{"Id": "img_race", "RepoTags": []string{"race:v1"}}
	st.imagesList = []map[string]any{{"Id": "img_race", "ParentID": ""}}
	st.inImageList = inCh
	st.blockImageList = blockCh
	st.mu.Unlock()

	var raceRes1 []ContainerInfo
	var raceErr1 error
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		raceRes1, raceErr1 = cli.ListContainers(ctx, true)
	}()

	<-inCh
	cli.InvalidateImageCache()
	st.mu.Lock()
	st.imagesList = []map[string]any{{"Id": "img_race", "ParentID": "sha256:fresh-parent"}}
	st.mu.Unlock()

	raceRes2, raceErr2 := cli.ListContainers(ctx, true)
	if raceErr2 != nil || len(raceRes2) != 1 || !raceRes2[0].IsLocalImage {
		t.Fatalf("fresh goroutine expected IsLocalImage=true, got %#v err=%v", raceRes2, raceErr2)
	}

	close(blockCh)
	<-done1
	if raceErr1 != nil || len(raceRes1) != 1 {
		t.Fatalf("stale goroutine unexpected error: %v", raceErr1)
	}

	metaAfter, ok := cli.getCachedImageMeta("img_race")
	if !ok || !metaAfter.ParentKnown || metaAfter.ParentID != "sha256:fresh-parent" {
		t.Fatalf("stale response overwrote fresh cached parent metadata: ok=%v meta=%#v", ok, metaAfter)
	}
}

func TestListContainersUsesManifestDescriptorPlatformWithInspectFallback(t *testing.T) {
	ctx := context.Background()

	containers := []map[string]any{
		{
			"Id":      "c-amd64",
			"Names":   []string{"/c-amd64"},
			"Image":   "repo/app:latest",
			"ImageID": "sha256:shared-multiarch",
			"State":   "running",
			"ImageManifestDescriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				"size":      100,
				"platform": map[string]any{
					"os":           "linux",
					"architecture": "amd64",
				},
			},
		},
		{
			"Id":      "c-arm64",
			"Names":   []string{"/c-arm64"},
			"Image":   "repo/app:latest",
			"ImageID": "sha256:shared-multiarch",
			"State":   "running",
			"ImageManifestDescriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
				"size":      100,
				"platform": map[string]any{
					"os":           "linux",
					"architecture": "arm64",
					"variant":      "v8",
				},
			},
		},
		{
			"Id":      "c-legacy",
			"Names":   []string{"/c-legacy"},
			"Image":   "repo/legacy:v1",
			"ImageID": "sha256:legacy-image",
			"State":   "running",
		},
	}

	inspects := map[string]map[string]any{
		"sha256:shared-multiarch": {
			"Id":           "sha256:shared-multiarch",
			"Os":           "linux",
			"Architecture": "amd64",
			"RepoTags":     []string{"repo/app:latest"},
			"RepoDigests":  []string{"repo/app@sha256:1111111111111111111111111111111111111111111111111111111111111111"},
		},
		"sha256:legacy-image": {
			"Id":           "sha256:legacy-image",
			"Os":           "linux",
			"Architecture": "s390x",
			"RepoTags":     []string{"repo/legacy:v1"},
			"RepoDigests":  []string{"repo/legacy@sha256:3333333333333333333333333333333333333333333333333333333333333333"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(containers)
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
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()

	cli := &Client{
		docker:       dockerCli,
		imageCache:   make(map[string]cachedImageMeta),
		startedCache: make(map[string]cachedStartMeta),
	}

	list, err := cli.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	byName := make(map[string]ContainerInfo, len(list))
	for _, c := range list {
		byName[c.Name] = c
	}
	if got := byName["c-amd64"]; got.ImageOS != "linux" || got.ImageArch != "amd64" || got.ImageVariant != "" {
		t.Fatalf("unexpected c-amd64 platform: %#v", got)
	}
	if got := byName["c-arm64"]; got.ImageOS != "linux" || got.ImageArch != "arm64" || got.ImageVariant != "v8" {
		t.Fatalf("expected c-arm64 platform linux/arm64/v8, got: %#v", got)
	}
	if got := byName["c-legacy"]; got.ImageOS != "linux" || got.ImageArch != "s390x" {
		t.Fatalf("expected c-legacy fallback platform linux/s390x, got: %#v", got)
	}
}
