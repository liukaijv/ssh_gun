package shellopen

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open opens path in the operating system's file manager.
func Open(path string) error {
	return openPath(path)
}

// Reveal opens the file manager and selects path when the OS supports it.
func Reveal(path string) error {
	return revealPath(path)
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

func revealCommandForOS(goos, path string) (string, []string) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{"/select," + path}
	case "darwin":
		return "open", []string{"-R", path}
	default:
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

func startCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// Used by non-Windows builds; also keeps GOOS switch testable.
func openPathViaCommand(path string) error {
	name, args := commandForOS(runtime.GOOS, path)
	return startCommand(name, args...)
}

func revealPathViaCommand(path string) error {
	name, args := revealCommandForOS(runtime.GOOS, path)
	return startCommand(name, args...)
}
