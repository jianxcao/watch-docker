package wsstream

import "testing"

func TestNewComposePullSourceUsesPrefixedHubKey(t *testing.T) {
	source := NewComposePullSource("/tmp/jellyfin", "jellyfin")

	if got, want := source.GetKey(), "compose-pull-jellyfin"; got != want {
		t.Fatalf("unexpected pull hub key: got %q, want %q", got, want)
	}
}
