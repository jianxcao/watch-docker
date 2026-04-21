package dockercli

import (
	"testing"

	"github.com/docker/docker/api/types/image"
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
