package config

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

func setupTempConfigEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origEnv := conf.EnvCfg
	origCfg := Get()
	conf.EnvCfg.CONFIG_PATH = dir
	conf.EnvCfg.CONFIG_FILE = "config.yaml"
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
		if origCfg != nil {
			globalMu.Lock()
			globalCfg = origCfg
			globalMu.Unlock()
		}
	})
	return filepath.Join(dir, "config.yaml")
}

func TestConfigCloneIsolation(t *testing.T) {
	orig := defaults()
	orig.Policy.SkipLabels = []string{"a=1"}
	orig.Registry.Auth = []RegistryAuth{{Host: "ghcr.io", Username: "u", Token: "secret"}}
	orig.TwoFAConfig.Users["admin"] = TwoFAUserConfig{
		Method:              "otp",
		OTPSecret:           "BASE32SECRET",
		WebAuthnCredentials: []string{"cred-1"},
	}

	cp := orig.Clone()
	cp.Policy.SkipLabels[0] = "mutated"
	cp.Registry.Auth[0].Token = "mutated"
	u := cp.TwoFAConfig.Users["admin"]
	u.WebAuthnCredentials[0] = "mutated"
	cp.TwoFAConfig.Users["admin"] = u

	if orig.Policy.SkipLabels[0] != "a=1" {
		t.Fatalf("expected SkipLabels isolated, got %v", orig.Policy.SkipLabels)
	}
	if orig.Registry.Auth[0].Token != "secret" {
		t.Fatalf("expected Registry.Auth isolated, got %q", orig.Registry.Auth[0].Token)
	}
	if orig.TwoFAConfig.Users["admin"].WebAuthnCredentials[0] != "cred-1" {
		t.Fatalf("expected TwoFAConfig isolated, got %v", orig.TwoFAConfig.Users["admin"].WebAuthnCredentials)
	}
}

func TestMaskedCopyAndMergeMaskedSecrets(t *testing.T) {
	prev := defaults()
	prev.Docker.Host = "unix:///custom/docker.sock"
	prev.Proxy.URL = "http://alice:proxy-pass@127.0.0.1:7890"
	prev.Registry.Auth = []RegistryAuth{
		{Host: "ghcr.io", Username: "alice", Token: "ghp_real_token"},
		{Host: "registry.example.com", Username: "bob", Token: "bob_token"},
	}
	prev.TwoFAConfig.Users["admin"] = TwoFAUserConfig{
		Method:    "otp",
		OTPSecret: "SECRET2FA",
	}

	masked := prev.MaskedCopy()
	if masked.Registry.Auth[0].Token != MaskedSecretPlaceholder {
		t.Fatalf("expected masked registry token, got %q", masked.Registry.Auth[0].Token)
	}
	if masked.Proxy.URL == prev.Proxy.URL {
		t.Fatalf("expected masked proxy password, got %q", masked.Proxy.URL)
	}
	if len(masked.TwoFAConfig.Users) != 0 {
		t.Fatalf("expected TwoFAConfig stripped in MaskedCopy, got %+v", masked.TwoFAConfig)
	}

	incoming := masked.Clone()
	incoming.Docker.Host = "tcp://evil:2375"
	incoming.Registry.Auth[1].Token = "new_bob_token"
	incoming.Policy.SkipLabels = nil
	incoming.TwoFAConfig = TwoFAConfig{Users: map[string]TwoFAUserConfig{}}

	MergeMaskedSecrets(incoming, prev)

	if incoming.Docker.Host != prev.Docker.Host {
		t.Fatalf("expected Docker.Host preserved %q, got %q", prev.Docker.Host, incoming.Docker.Host)
	}
	if incoming.Registry.Auth[0].Token != "ghp_real_token" {
		t.Fatalf("expected placeholder token restored, got %q", incoming.Registry.Auth[0].Token)
	}
	if incoming.Registry.Auth[1].Token != "new_bob_token" {
		t.Fatalf("expected updated token retained, got %q", incoming.Registry.Auth[1].Token)
	}
	if incoming.Proxy.URL != "http://alice:proxy-pass@127.0.0.1:7890" {
		t.Fatalf("expected proxy password restored, got %q", incoming.Proxy.URL)
	}
	if incoming.TwoFAConfig.Users["admin"].OTPSecret != "SECRET2FA" {
		t.Fatalf("expected TwoFAConfig preserved, got %+v", incoming.TwoFAConfig)
	}
	if incoming.Policy.SkipLabels == nil {
		t.Fatalf("expected nil slice normalized to empty slice")
	}
}

func TestUpdateAtomicPermissionsAndConcurrency(t *testing.T) {
	cfgPath := setupTempConfigEnv(t)
	if _, err := Load(cfgPath); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 permissions, got %#o", perm)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = Update(func(cfg *Config) error {
					cfg.Notify.Method = http.MethodPost
					cfg.Scan.Concurrency = (idx % 4) + 1
					return nil
				})
				_ = Get()
			}
		}(i)
	}
	wg.Wait()
}
