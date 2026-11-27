package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

func TestLocalTestEffectiveConfig(t *testing.T) {
	t.Run("parseCLIArgs", func(t *testing.T) {
		if lt, err := parseCLIArgs(nil); err != nil || lt {
			t.Fatalf("empty args expected (false, nil), got (%v, %v)", lt, err)
		}
		if lt, err := parseCLIArgs([]string{"--local-test"}); err != nil || !lt {
			t.Fatalf("--local-test expected (true, nil), got (%v, %v)", lt, err)
		}
	})

	t.Run("first startup without password rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CONFIG_PATH", dir)
		t.Setenv("USER_NAME", "admin")
		t.Setenv("USER_PASSWORD", "")
		t.Setenv("IS_OPEN_DOCKER_SHELL", "false")

		// 空 USER_PASSWORD 与未设置 PASSWORD 均按默认/空白拒绝
		cfg := conf.NewEnvConfig()
		if err := validateLocalTestConfig(cfg, false); err == nil {
			t.Fatal("expected validateLocalTestConfig to fail when password is empty or default")
		}
	})

	t.Run("unset USER_PASSWORD falls back to default password and is rejected", func(t *testing.T) {
		if orig, had := os.LookupEnv("USER_PASSWORD"); had {
			t.Cleanup(func() { _ = os.Setenv("USER_PASSWORD", orig) })
			_ = os.Unsetenv("USER_PASSWORD")
		}
		dir := t.TempDir()
		t.Setenv("CONFIG_PATH", dir)
		t.Setenv("USER_NAME", "admin")

		cfg := conf.NewEnvConfig()
		if err := validateLocalTestConfig(cfg, false); err == nil {
			t.Fatal("expected default password on first startup to fail validateLocalTestConfig")
		}
	})

	t.Run("env custom password succeeds", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CONFIG_PATH", dir)
		t.Setenv("USER_NAME", "tester")
		t.Setenv("USER_PASSWORD", "LocalStrongPass!2026")

		cfg := conf.NewEnvConfig()
		if err := validateLocalTestConfig(cfg, isEnvExplicitTrue("IS_OPEN_DOCKER_SHELL")); err != nil {
			t.Fatalf("expected valid env password to pass, got: %v", err)
		}
	})

	t.Run("empty username rejected without leaking password", func(t *testing.T) {
		secret := "SecretPassShouldNotAppear999!"
		dir := t.TempDir()
		t.Setenv("CONFIG_PATH", dir)
		t.Setenv("USER_NAME", "   ")
		t.Setenv("USER_PASSWORD", secret)

		cfg := conf.NewEnvConfig()
		err := validateLocalTestConfig(cfg, false)
		if err == nil {
			t.Fatal("expected empty username to fail")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error message must not contain password")
		}
	})

	t.Run("disk app.yaml with default password rejected even when env sets password", func(t *testing.T) {
		dir := t.TempDir()
		appYaml := filepath.Join(dir, "app.yaml")
		if err := os.WriteFile(appYaml, []byte("username: admin\npassword: admin\nenable_docker_shell: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CONFIG_PATH", dir)
		t.Setenv("USER_PASSWORD", "EnvPasswordOverriddenByDisk!")

		cfg := conf.NewEnvConfig()
		err := validateLocalTestConfig(cfg, false)
		if err == nil {
			t.Fatal("expected disk app.yaml default password to fail")
		}
	})

	t.Run("disk app.yaml shell enabled without explicit IS_OPEN_DOCKER_SHELL rejected", func(t *testing.T) {
		dir := t.TempDir()
		secret := "DiskSecretPass!888"
		appYaml := filepath.Join(dir, "app.yaml")
		content := "username: admin\npassword: " + secret + "\nenable_docker_shell: true\n"
		if err := os.WriteFile(appYaml, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CONFIG_PATH", dir)
		if err := validateLocalTestConfig(conf.NewEnvConfig(), isEnvExplicitTrue("IS_OPEN_DOCKER_SHELL")); err == nil {
			t.Fatal("expected effective enable_docker_shell=true without explicit env opt-in to fail")
		}

		// 显式设置 IS_OPEN_DOCKER_SHELL=true 后通过
		t.Setenv("IS_OPEN_DOCKER_SHELL", "true")
		if err := validateLocalTestConfig(conf.NewEnvConfig(), isEnvExplicitTrue("IS_OPEN_DOCKER_SHELL")); err != nil {
			t.Fatalf("expected explicit IS_OPEN_DOCKER_SHELL=true to allow disk shell config, got: %v", err)
		}
	})

	t.Run("disk app.yaml custom password succeeds without env USER_PASSWORD", func(t *testing.T) {
		if orig, had := os.LookupEnv("USER_PASSWORD"); had {
			t.Cleanup(func() { _ = os.Setenv("USER_PASSWORD", orig) })
			_ = os.Unsetenv("USER_PASSWORD")
		}
		dir := t.TempDir()
		secret := "DiskConfiguredSecret!2026"
		appYaml := filepath.Join(dir, "app.yaml")
		if err := os.WriteFile(appYaml, []byte("username: custom-admin\npassword: "+secret+"\nenable_docker_shell: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CONFIG_PATH", dir)

		cfg := conf.NewEnvConfig()
		if err := validateLocalTestConfig(cfg, false); err != nil {
			t.Fatalf("expected disk app.yaml custom password to succeed, got: %v", err)
		}
	})
}
