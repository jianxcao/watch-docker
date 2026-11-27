package policy

import "testing"

func TestEvaluateSkipLocalUsesExplicitLocalFlag(t *testing.T) {
	dec := Evaluate(Input{
		ImageRef:     "nginx:latest",
		RepoDigests:  nil,
		SkipLocal:    true,
		IsLocalImage: false,
	})

	if dec.Skipped {
		t.Fatalf("expected remote image without local flag to remain eligible, got skipped: %+v", dec)
	}
}

func TestEvaluateSkipLocalStillSkipsExplicitLocalImage(t *testing.T) {
	dec := Evaluate(Input{
		ImageRef:     "local/app:latest",
		RepoDigests:  nil,
		SkipLocal:    true,
		IsLocalImage: true,
	})

	if !dec.Skipped || dec.Reason != "local build" {
		t.Fatalf("expected explicit local image to be skipped, got %+v", dec)
	}
}

func TestImageTagWithRegistryPort(t *testing.T) {
	if got := imageTag("192.168.1.10:5000/org/app"); got != "latest" {
		t.Fatalf("expected latest for untagged port registry ref, got %q", got)
	}
	if got := imageTag("192.168.1.10:5000/org/app:v1.2.3"); got != "v1.2.3" {
		t.Fatalf("expected v1.2.3, got %q", got)
	}
	if got := imageTag("nginx@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); got != "" {
		t.Fatalf("expected empty tag for digest-only ref, got %q", got)
	}

	dec := Evaluate(Input{
		ImageRef:     "192.168.1.10:5000/org/app",
		FloatingTags: []string{"latest"},
	})
	if dec.Skipped {
		t.Fatalf("expected untagged port-qualified ref to match floating 'latest', got %+v", dec)
	}
}
