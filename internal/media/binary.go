package media

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// CommandExecutor is a cross-platform wrapper for command execution with clean cancellation
type CommandExecutor struct {
	cmd *exec.Cmd
}

func NewExecutor(ctx context.Context, name string, args ...string) *CommandExecutor {
	cmd := exec.CommandContext(ctx, name, args...)
	setupProcessGroup(cmd)
	return &CommandExecutor{cmd: cmd}
}

func (e *CommandExecutor) Cmd() *exec.Cmd {
	return e.cmd
}

// Start wraps cmd.Start()
func (e *CommandExecutor) Start() error {
	return e.cmd.Start()
}

// Wait wraps cmd.Wait()
func (e *CommandExecutor) Wait() error {
	err := e.cmd.Wait()
	// Always cleanup the group just in case
	e.Cleanup()
	return err
}

// Run is a helper for Start + Wait
func (e *CommandExecutor) Run() error {
	if err := e.Start(); err != nil {
		return err
	}
	return e.Wait()
}

func ResolveBinary(customPath string, names ...string) string {
	if strings.TrimSpace(customPath) != "" {
		if _, err := os.Stat(customPath); err == nil {
			return customPath
		}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
