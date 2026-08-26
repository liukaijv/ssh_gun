package syncengine

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"
)

// isReparsePoint reports symlinks, Windows junctions, and PHP symlink() targets
// that should not be opened or walked during sync.
func isReparsePoint(entry fs.DirEntry, path string) bool {
	if entry != nil && entry.Type()&fs.ModeSymlink != 0 {
		return true
	}
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return pathHasReparsePoint(path)
}

// IsLocalReparsePoint reports local paths that must not be uploaded or mkdir'd.
func IsLocalReparsePoint(path string) bool {
	return isReparsePoint(nil, path)
}

// IsInaccessibleLocalError reports open/stat failures typical of PHP symlinks and
// junctions on Windows that cannot be read as normal files.
func IsInaccessibleLocalError(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 1920 // ERROR_CANT_ACCESS_FILE
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errno, ok := pathErr.Err.(syscall.Errno); ok {
			return errno == 1920
		}
	}
	return strings.Contains(err.Error(), "cannot be accessed by the system")
}
