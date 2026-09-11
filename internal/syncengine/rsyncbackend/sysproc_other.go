//go:build !windows

package rsyncbackend

import "os/exec"

func configureRsyncCmd(cmd *exec.Cmd) {}
