//go:build windows

package tui

import "os/exec"

func configureGitActionProcess(cmd *exec.Cmd) {
}

func killGitActionProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
