//go:build windows

package procman

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureSysProcAttr(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow,
	}
}

func killProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	// Prevent a brief console window flash when Feisuo is a GUI app.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill pid %d: %w (%s)", pid, err, strings.TrimSpace(string(out)))
	}
	return nil
}
