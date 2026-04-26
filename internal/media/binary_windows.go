//go:build windows

package media

import (
	"os/exec"
	"strconv"
)

func setupProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't use pgid in the same way as Unix for exec.Cmd
}

func (e *CommandExecutor) Cleanup() {
	if e.cmd == nil || e.cmd.Process == nil {
		return
	}
	// Use taskkill to kill the process and all its children (/T)
	// /F is force
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(e.cmd.Process.Pid)).Run()
}
