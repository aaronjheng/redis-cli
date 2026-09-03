//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package ssh

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// hasControllingTTY reports whether the process has a controlling terminal.
// ssh(1) reads interactive prompts from /dev/tty, which only works when the
// child process shares the foreground process group.
func hasControllingTTY() bool {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}

	_ = file.Close()

	return true
}

func configureProcessGroup(cmd *exec.Cmd, separateGroup bool) {
	if separateGroup {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
}

func terminateProcessGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("syscall.Kill error: %w", err)
	}

	return nil
}

func killProcessGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("syscall.Kill error: %w", err)
	}

	return nil
}

func terminateProcess(cmd *exec.Cmd) error {
	err := cmd.Process.Signal(syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("cmd.Process.Signal error: %w", err)
	}

	return nil
}

func killProcess(cmd *exec.Cmd) error {
	err := cmd.Process.Kill()
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("cmd.Process.Kill error: %w", err)
	}

	return nil
}
