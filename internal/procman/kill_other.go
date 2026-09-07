//go:build !windows

package procman

import (
	"fmt"
	"os"
	"os/exec"
)

func configureSysProcAttr(cmd *exec.Cmd) {}

func killProcessTree(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	return nil
}
