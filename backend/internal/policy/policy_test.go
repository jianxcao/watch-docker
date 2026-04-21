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
