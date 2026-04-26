//go:build !windows

package media

import (
	"os/exec"
	"syscall"
)

func setupProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func (e *CommandExecutor) Cleanup() {
	if e.cmd == nil || e.cmd.Process == nil {
		return
	}
	// Kill the entire process group
	_ = syscall.Kill(-e.cmd.Process.Pid, syscall.SIGKILL)
}
