package api

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

const (
	terminalTargetLocal = "local"
	terminalTargetHost  = "host"
	hostTargetRoot      = "/proc/1/root"
)

var knownShellPaths = []string{
	"/bin/sh",
	"/bin/bash",
	"/bin/zsh",
	"/bin/ash",
	"/usr/bin/bash",
	"/usr/bin/zsh",
}

type ShellOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ShellOptions struct {
	Shells       []ShellOption `json:"shells"`
	DefaultShell string        `json:"defaultShell"`
	Target       string        `json:"target"`
}

type shellCommand struct {
	Path string
	Args []string
}

func discoverShells(root, defaultShell string) ShellOptions {
	options := ShellOptions{Shells: make([]ShellOption, 0, len(knownShellPaths))}
	for _, shellPath := range knownShellPaths {
		fullPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(shellPath, "/")))
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		options.Shells = append(options.Shells, ShellOption{
			Label: filepath.Base(shellPath),
			Value: shellPath,
		})
	}

	for _, option := range options.Shells {
		if option.Value == defaultShell {
			options.DefaultShell = defaultShell
			return options
		}
	}
	if len(options.Shells) > 0 {
		options.DefaultShell = options.Shells[0].Value
	}
	return options
}

func resolveShellCommand(target, selectedShell string, options ShellOptions, nsenterPath string) (shellCommand, error) {
	if target != terminalTargetLocal && target != terminalTargetHost {
		return shellCommand{}, fmt.Errorf("不支持的终端目标: %s", target)
	}
	if !containsShell(options.Shells, selectedShell) {
		return shellCommand{}, fmt.Errorf("Shell 不可用: %s", selectedShell)
	}
	if target == terminalTargetLocal {
		return shellCommand{Path: selectedShell}, nil
	}
	if nsenterPath == "" {
		return shellCommand{}, errors.New("未找到 nsenter，请在镜像中安装 util-linux")
	}
	return shellCommand{
		Path: nsenterPath,
		Args: []string{
			"--target", "1",
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--root", "--wdns=/", "--", selectedShell,
		},
	}, nil
}

func containsShell(options []ShellOption, selectedShell string) bool {
	for _, option := range options {
		if option.Value == selectedShell {
			return true
		}
	}
	return false
}

func validateHostRoot(currentRoot, targetRoot string) error {
	currentInfo, err := os.Stat(currentRoot)
	if err != nil {
		return fmt.Errorf("读取当前容器根目录失败: %w", err)
	}
	targetInfo, err := os.Stat(targetRoot)
	if err != nil {
		return fmt.Errorf("读取宿主机根目录失败，请配置 pid: host: %w", err)
	}
	if os.SameFile(currentInfo, targetInfo) {
		return errors.New("未检测到独立的宿主机根目录，请为 Watch Docker 配置 pid: host")
	}
	return nil
}

func configuredTerminalTarget() (string, error) {
	target := strings.ToLower(strings.TrimSpace(conf.EnvCfg.TERMINAL_TARGET))
	if target == "" {
		target = terminalTargetLocal
	}
	if target != terminalTargetLocal && target != terminalTargetHost {
		return "", fmt.Errorf("不支持的 TERMINAL_TARGET: %s", target)
	}
	return target, nil
}
