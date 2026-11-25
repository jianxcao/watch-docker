package scanner

import "testing"

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
