package syncengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsInaccessibleLocalError(t *testing.T) {
	if !IsInaccessibleLocalError(errors.New(`open foo: The file cannot be accessed by the system.`)) {
		t.Fatal("expected inaccessible local error")
	}
	if IsInaccessibleLocalError(os.ErrNotExist) {
		t.Fatal("unexpected match for ErrNotExist")
	}
}

func TestScanLocal_SkipsReparsePoint(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	entries, err := ScanLocal(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.RelPath == "link" || entry.RelPath == "link/ok.txt" {
			t.Fatalf("reparse point was scanned: %#v", entries)
		}
	}
}
