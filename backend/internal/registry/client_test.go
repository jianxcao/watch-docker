package registry

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
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
