package dockercli

import (
	"context"
	"io"
	"strings"
	"sync"
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
