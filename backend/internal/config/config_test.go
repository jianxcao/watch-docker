package config

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestConfigLoadPortOverrideEmptyYAMLAndViperEnv(t *testing.T) {
	cfgPath := setupTempConfigEnv(t)

	// 写入 0 字节文件与旧 addr 测试
	if err := os.WriteFile(cfgPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "19090")
	t.Setenv("WATCH_SCAN_CONCURRENCY", "7")

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load on empty YAML failed: %v", err)
	}
	if loaded.Server.Addr != ":19090" {
		t.Fatalf("expected Server.Addr=:19090 from PORT, got %q", loaded.Server.Addr)
	}
	if loaded.Scan.Concurrency != 7 {
		t.Fatalf("expected Scan.Concurrency=7 from env on empty YAML, got %d", loaded.Scan.Concurrency)
	}
	// 验证再次重启带新 PORT 时能覆盖已写入的 server.addr
	t.Setenv("PORT", "28080")
	loaded2, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("second Load with new PORT failed: %v", err)
	}
	if loaded2.Server.Addr != ":28080" {
		t.Fatalf("expected new PORT=:28080 to override persisted addr, got %q", loaded2.Server.Addr)
	}
}

func TestMergeMaskedSecretsStrictMatchingAndValidate(t *testing.T) {
	prev := defaults()
	prev.Proxy.URL = "http://alice:proxy-pass@127.0.0.1:7890"
	prev.Notify.URL = "https://user:notify-pass@notify.example.com/hook"
	prev.Registry.Auth = []RegistryAuth{
		{Host: "ghcr.io", Username: "alice", Token: "secret-a"},
	}

	masked := prev.MaskedCopy()
	if masked.Notify.URL == prev.Notify.URL || !strings.Contains(masked.Notify.URL, "user:") {
		t.Fatalf("expected Notify.URL password masked while keeping username, got %q", masked.Notify.URL)
	}

	// 修改 Proxy 主机名时不应恢复旧密码，且 Validate 必须拦截占位符
	changedProxy := masked.Clone()
	changedProxy.Proxy.URL = "http://alice:" + MaskedSecretPlaceholder + "@evil.example:7890"
	MergeMaskedSecrets(changedProxy, prev)
	if !strings.Contains(changedProxy.Proxy.URL, MaskedSecretPlaceholder) && !strings.Contains(changedProxy.Proxy.URL, "%2A%2A%2A%2A%2A%2A") {
		// unchanged placeholder should not become "proxy-pass"
	}
	if strings.Contains(changedProxy.Proxy.URL, "proxy-pass") {
		t.Fatalf("must not restore proxy password when host changed: %q", changedProxy.Proxy.URL)
	}
	if err := Validate(changedProxy); err == nil {
		t.Fatalf("expected Validate to reject un-updated masked proxy password")
	}

	// 修改 Registry.Auth 用户名时不能按切片索引误恢复旧 Token
	changedReg := masked.Clone()
	changedReg.Registry.Auth[0].Username = "bob"
	MergeMaskedSecrets(changedReg, prev)
	if changedReg.Registry.Auth[0].Token == "secret-a" {
		t.Fatalf("must not restore registry token across different username")
	}
	if err := Validate(changedReg); err == nil {
		t.Fatalf("expected Validate to reject un-updated masked registry token")
	}

	// Cron 与 URL scheme 校验
	badCron := defaults()
	badCron.Scan.Cron = "not-a-cron"
	if err := Validate(badCron); err == nil {
		t.Fatalf("expected Validate to reject invalid cron expression")
	}
	goodCron6 := defaults()
	goodCron6.Scan.Cron = "0 */5 * * * *"
	if err := Validate(goodCron6); err != nil {
		t.Fatalf("expected 6-field cron to validate, got %v", err)
	}
	badProxyScheme := defaults()
	badProxyScheme.Proxy.URL = "file:///etc/passwd"
	if err := Validate(badProxyScheme); err == nil {
		t.Fatalf("expected Validate to reject file:// proxy scheme")
	}
	goodSocksProxy := defaults()
	goodSocksProxy.Proxy.URL = "socks5://127.0.0.1:1080"
	if err := Validate(goodSocksProxy); err != nil {
		t.Fatalf("expected socks5 proxy to validate, got %v", err)
	}

	// Logging.Level 校验与默认规范化
	emptyLevel := defaults()
	emptyLevel.Logging.Level = "  "
	if err := Validate(emptyLevel); err != nil {
		t.Fatalf("expected empty logging.level to default cleanly, got %v", err)
	}
	if emptyLevel.Logging.Level != "info" {
		t.Fatalf("expected normalized logging.level 'info', got %q", emptyLevel.Logging.Level)
	}
	badLevel := defaults()
	badLevel.Logging.Level = "trace"
	if err := Validate(badLevel); err == nil {
		t.Fatalf("expected Validate to reject unsupported logging.level")
	}
}

func TestDurationMinutesYAMLAndEnvDecode(t *testing.T) {
	cfgPath := setupTempConfigEnv(t)
	if err := os.WriteFile(cfgPath, []byte("scan:\n  cacheTTL: 60\n"), 0600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load cacheTTL=60 failed: %v", err)
	}
	if loaded.Scan.CacheTTL.Duration() != 60*time.Minute {
		t.Fatalf("expected cacheTTL 60m, got %v", loaded.Scan.CacheTTL.Duration())
	}

	// 验证 env WATCH_SCAN_CACHETTL 覆盖（分钟）与重新落盘后仍为分钟整数
	t.Setenv("WATCH_SCAN_CACHETTL", "45")
	loadedEnv, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load env WATCH_SCAN_CACHETTL=45 failed: %v", err)
	}
	if loadedEnv.Scan.CacheTTL.Duration() != 45*time.Minute {
		t.Fatalf("expected cacheTTL 45m, got %v", loadedEnv.Scan.CacheTTL.Duration())
	}

	SetGlobal(loadedEnv)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "cachettl: 45") {
		t.Fatalf("expected persisted cachettl: 45, got:\n%s", string(raw))
	}
	t.Setenv("WATCH_SCAN_CACHETTL", "")
	reloaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("reload persisted cachettl=45 failed: %v", err)
	}
	if reloaded.Scan.CacheTTL.Duration() != 45*time.Minute {
		t.Fatalf("expected reloaded cacheTTL 45m, got %v", reloaded.Scan.CacheTTL.Duration())
	}

	// 验证历史纳秒数值配置平滑升级
	if err := os.WriteFile(cfgPath, []byte("scan:\n  cacheTTL: 1800000000000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCH_SCAN_CACHETTL", "")
	loadedLegacy, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load legacy ns cacheTTL failed: %v", err)
	}
	if loadedLegacy.Scan.CacheTTL.Duration() != 30*time.Minute {
		t.Fatalf("expected legacy 1800000000000ns -> 30m, got %v", loadedLegacy.Scan.CacheTTL.Duration())
	}
}

func TestLegacyNotifyURLAtPrefixCompatibility(t *testing.T) {
	cfgPath := setupTempConfigEnv(t)
	legacyYAML := "notify:\n  url: \"@https://notify.example/hook?title={title}\"\n"
	if err := os.WriteFile(cfgPath, []byte(legacyYAML), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load legacy @https notify.url failed: %v", err)
	}
	want := "https://notify.example/hook?title={title}"
	if cfg.Notify.URL != want {
		t.Fatalf("notify URL = %q, want %q", cfg.Notify.URL, want)
	}

	diskRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(diskRaw) != legacyYAML {
		t.Fatalf("expected non-empty disk config untouched on Load, got:\n%s", string(diskRaw))
	}

	// 验证带用户名密码和占位符的 URL 经 MaskedCopy -> MergeMaskedSecrets -> Validate 后可完整还原
	prev := defaults()
	prev.Notify.URL = "@https://user:secret@notify.example/{title}?text={content}"
	if err := Validate(prev); err != nil {
		t.Fatalf("Validate prev notify.url failed: %v", err)
	}
	masked := prev.MaskedCopy()
	if strings.Contains(masked.Notify.URL, "secret") || !strings.Contains(masked.Notify.URL, "{title}") ||
		(!strings.Contains(masked.Notify.URL, MaskedSecretPlaceholder) && !strings.Contains(masked.Notify.URL, "%2A%2A%2A%2A%2A%2A")) {
		t.Fatalf("unexpected masked Notify.URL: %q", masked.Notify.URL)
	}

	incoming := masked.Clone()
	incoming.Notify.URL = "@" + masked.Notify.URL
	MergeMaskedSecrets(incoming, prev)
	if err := Validate(incoming); err != nil {
		t.Fatalf("Validate restored Notify.URL failed: %v", err)
	}
	wantAuth := "https://user:secret@notify.example/{title}?text={content}"
	if incoming.Notify.URL != wantAuth {
		t.Fatalf("restored Notify.URL = %q, want %q", incoming.Notify.URL, wantAuth)
	}

	// 非法格式必须继续被 Validate 拒绝
	for _, bad := range []string{"@", "@@https://notify.example/hook", "@ftp://notify.example/hook"} {
		badCfg := defaults()
		badCfg.Notify.URL = bad
		if err := Validate(badCfg); err == nil {
			t.Fatalf("expected Validate to reject %q", bad)
		}
	}
}
