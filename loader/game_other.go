//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func launchGame(dir string, port int) (*exec.Cmd, error) {
	return nil, fmt.Errorf("目前只支持 Windows")
}

func openFolder(dir string) error {
	return fmt.Errorf("目前只支持 Windows")
}

func hiddenCommand(name string, args ...string) (string, error) {
	return "", fmt.Errorf("目前只支持 Windows")
}
