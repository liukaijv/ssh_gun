//go:build windows

package rsyncbackend

import (
	"os/exec"
	"syscall"
)

func configureRsyncCmd(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
