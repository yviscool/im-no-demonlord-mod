//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func launchGame(dir string, port int) (*exec.Cmd, error) {
	exe := filepath.Join(dir, exeName)
	if _, err := os.Stat(exe); err != nil {
		return nil, fmt.Errorf("找不到游戏程序：%s", exe)
	}
	// Keep the renderer running (and screenshots/visibility working) even when
	// the game window is behind other windows.
	cmd := exec.Command(exe,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-background-timer-throttling",
	)
	cmd.Dir = dir
	// Electron is a GUI program: it opens its own window and never allocates a
	// console. Passing HideWindow here would make Chromium create its first
	// window hidden (STARTUPINFO show state), so leave the show state alone.
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func openFolder(dir string) error {
	return exec.Command("explorer", dir).Start()
}

// hiddenCommand runs a console program without showing a console window.
func hiddenCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	return string(out), err
}
