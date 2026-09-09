// Package autostart manages launch-at-login via the OS startup mechanism.
package autostart

import (
	"fmt"
	"os"
	"strings"
)

// ValueName is the registry / startup entry name for Feisuo.
const ValueName = "Feisuo"

// quoteExecutable returns a command line suitable for Run registry values.
func quoteExecutable(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.ContainsAny(path, " \t") {
		return `"` + path + `"`
	}
	return path
}

func executableCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	return quoteExecutable(exe), nil
}
