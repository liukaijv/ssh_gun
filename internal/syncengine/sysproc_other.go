//go:build !windows

package syncengine

import "os/exec"

func configureHiddenCmd(cmd *exec.Cmd) {}
