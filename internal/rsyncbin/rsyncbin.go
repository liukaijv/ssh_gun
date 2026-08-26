package rsyncbin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const Version = "6.4.8"

// Extract installs the embedded cwRsync bin and etc directories below
// LOCALAPPDATA. A matching marker makes repeated calls inexpensive.
func Extract(source fs.FS, sourceRoot, localAppData string) (string, error) {
	if source == nil {
		return "", errors.New("cwrsync source filesystem is required")
	}
	if localAppData == "" {
		return "", errors.New("LOCALAPPDATA directory is required")
	}
	runtimeDir := filepath.Join(localAppData, "ssh_gun", "runtime", "cwrsync-"+Version)
	marker := filepath.Join(runtimeDir, ".version")
	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == Version {
		return runtimeDir, nil
	}

	if err := os.RemoveAll(runtimeDir); err != nil {
		return "", fmt.Errorf("remove old cwrsync runtime: %w", err)
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", fmt.Errorf("create cwrsync runtime: %w", err)
	}
	for _, directory := range []string{"bin", "etc"} {
		root := joinFSPath(sourceRoot, directory)
		if err := copyTree(source, root, filepath.Join(runtimeDir, directory)); err != nil {
			_ = os.RemoveAll(runtimeDir)
			return "", fmt.Errorf("extract cwrsync %s: %w", directory, err)
		}
	}
	if err := os.WriteFile(marker, []byte(Version), 0o644); err != nil {
		_ = os.RemoveAll(runtimeDir)
		return "", fmt.Errorf("write cwrsync version marker: %w", err)
	}
	return runtimeDir, nil
}

func copyTree(source fs.FS, sourceRoot, targetRoot string) error {
	return fs.WalkDir(source, sourceRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(filepath.FromSlash(sourceRoot), filepath.FromSlash(name))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid embedded path %q", name)
		}
		target := filepath.Join(targetRoot, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		input, err := source.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func joinFSPath(root, name string) string {
	root = strings.Trim(strings.ReplaceAll(root, `\`, "/"), "/")
	if root == "" || root == "." {
		return name
	}
	return root + "/" + name
}

// Cygpath converts Windows drive paths to cwRsync's /cygdrive form.
func Cygpath(windowsPath string) string {
	normalized := strings.ReplaceAll(windowsPath, `\`, "/")
	if len(normalized) >= 2 && normalized[1] == ':' && unicode.IsLetter(rune(normalized[0])) {
		drive := unicode.ToLower(rune(normalized[0]))
		rest := strings.TrimPrefix(normalized[2:], "/")
		if rest == "" {
			return fmt.Sprintf("/cygdrive/%c", drive)
		}
		suffix := ""
		if strings.HasSuffix(normalized, "/") {
			suffix = "/"
			rest = strings.TrimSuffix(rest, "/")
		}
		return fmt.Sprintf("/cygdrive/%c/%s%s", drive, rest, suffix)
	}
	return normalized
}
