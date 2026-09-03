//go:build windows

package ssh

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func hasControllingTTY() bool {
	return false
}

func configureProcessGroup(_ *exec.Cmd, _ bool) {}

func terminateProcessGroup(cmd *exec.Cmd) error {
	return killProcess(cmd)
}

func killProcessGroup(cmd *exec.Cmd) error {
	return killProcess(cmd)
}

func terminateProcess(cmd *exec.Cmd) error {
	return killProcess(cmd)
}

func killProcess(cmd *exec.Cmd) error {
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("cmd.Process.Kill error: %w", err)
	}

	return nil
}
