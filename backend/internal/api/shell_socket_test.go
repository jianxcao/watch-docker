package api

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type concurrentWriteProbe struct {
	active  atomic.Int32
	overlap atomic.Bool
}

func (p *concurrentWriteProbe) SetWriteDeadline(time.Time) error { return nil }

func (p *concurrentWriteProbe) WriteMessage(int, []byte) error {
	if p.active.Add(1) > 1 {
		p.overlap.Store(true)
	}
	time.Sleep(time.Millisecond)
	p.active.Add(-1)
	return nil
}

func TestSynchronizedWebSocketWriterSerializesWrites(t *testing.T) {
	probe := &concurrentWriteProbe{}
	writer := &synchronizedWebSocketWriter{conn: probe}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := writer.writeMessage(websocket.BinaryMessage, []byte("output")); err != nil {
				t.Errorf("write message: %v", err)
			}
		}()
	}
	wg.Wait()
	if probe.overlap.Load() {
		t.Fatal("expected websocket writes to be serialized")
	}
}

func TestDiscoverShellsReturnsOnlyExecutableKnownShells(t *testing.T) {
	root := t.TempDir()
	writeShellFixture(t, root, "/bin/sh", 0o755)
	writeShellFixture(t, root, "/bin/bash", 0o644)
	writeShellFixture(t, root, "/bin/zsh", 0o755)

	got := discoverShells(root, "/bin/zsh")
	wantShells := []ShellOption{
		{Label: "sh", Value: "/bin/sh"},
		{Label: "zsh", Value: "/bin/zsh"},
	}
	if !reflect.DeepEqual(got.Shells, wantShells) {
		t.Fatalf("unexpected shell options: got %#v want %#v", got.Shells, wantShells)
	}
	if got.DefaultShell != "/bin/zsh" {
		t.Fatalf("expected executable configured default, got %q", got.DefaultShell)
	}
}

func TestDiscoverShellsFallsBackToFirstExecutableShell(t *testing.T) {
	root := t.TempDir()
	writeShellFixture(t, root, "/bin/bash", 0o755)

	got := discoverShells(root, "/bin/missing")
	if got.DefaultShell != "/bin/bash" {
		t.Fatalf("expected first executable shell fallback, got %q", got.DefaultShell)
	}
}

func TestResolveShellCommandBuildsLocalCommand(t *testing.T) {
	options := ShellOptions{Shells: []ShellOption{{Label: "sh", Value: "/bin/sh"}}}

	got, err := resolveShellCommand("local", "/bin/sh", options, "/usr/bin/nsenter")
	if err != nil {
		t.Fatalf("resolve local shell: %v", err)
	}
	if got.Path != "/bin/sh" || len(got.Args) != 0 {
		t.Fatalf("unexpected local command: %#v", got)
	}
}

func TestResolveShellCommandBuildsHostNsenterCommand(t *testing.T) {
	options := ShellOptions{Shells: []ShellOption{{Label: "bash", Value: "/bin/bash"}}}

	got, err := resolveShellCommand("host", "/bin/bash", options, "/usr/bin/nsenter")
	if err != nil {
		t.Fatalf("resolve host shell: %v", err)
	}
	wantArgs := []string{
		"--target", "1",
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--root", "--wdns=/", "--", "/bin/bash",
	}
	if got.Path != "/usr/bin/nsenter" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("unexpected host command: got %#v want path %q args %#v", got, "/usr/bin/nsenter", wantArgs)
	}
}

func TestResolveShellCommandRejectsUndiscoveredShell(t *testing.T) {
	options := ShellOptions{Shells: []ShellOption{{Label: "sh", Value: "/bin/sh"}}}

	if _, err := resolveShellCommand("host", "/bin/bash", options, "/usr/bin/nsenter"); err == nil {
		t.Fatal("expected undiscovered shell to be rejected")
	}
}

func TestResolveShellCommandRejectsUnsupportedTarget(t *testing.T) {
	options := ShellOptions{Shells: []ShellOption{{Label: "sh", Value: "/bin/sh"}}}

	if _, err := resolveShellCommand("remote", "/bin/sh", options, "/usr/bin/nsenter"); err == nil {
		t.Fatal("expected unsupported terminal target to be rejected")
	}
}

func TestValidateHostRootRejectsCurrentContainerRoot(t *testing.T) {
	root := t.TempDir()
	if err := validateHostRoot(root, root); err == nil {
		t.Fatal("expected identical current and target roots to be rejected")
	}
}

func TestValidateHostRootAcceptsDifferentTargetRoot(t *testing.T) {
	currentRoot := t.TempDir()
	targetRoot := t.TempDir()
	if err := validateHostRoot(currentRoot, targetRoot); err != nil {
		t.Fatalf("expected distinct host target root: %v", err)
	}
}

func writeShellFixture(t *testing.T, root, shellPath string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(shellPath[1:]))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create shell fixture directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatalf("write shell fixture: %v", err)
	}
}
