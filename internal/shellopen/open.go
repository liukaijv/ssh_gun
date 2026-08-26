package shellopen

import (
	"os/exec"
	"runtime"
)

// Open opens path in the operating system's file manager.
func Open(path string) error {
	name, args := commandForOS(runtime.GOOS, path)
	return exec.Command(name, args...).Start()
}

func commandForOS(goos, path string) (string, []string) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{path}
	case "darwin":
		return "open", []string{path}
	default:
		return "xdg-open", []string{path}
	}
}
