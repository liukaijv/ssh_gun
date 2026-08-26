package syncengine

import (
	"fmt"
	"path/filepath"
	"strings"
)

func ToSlashRel(root, abs string) (string, error) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside root %q", abs, root)
	}
	return filepath.ToSlash(rel), nil
}
