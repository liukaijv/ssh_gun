package syncengine

import (
	"path/filepath"
	"testing"
)

func TestToSlashRel(t *testing.T) {
	root := filepath.Join("tmp", "project")
	abs := filepath.Join(root, "nested", "file.txt")
	got, err := ToSlashRel(root, abs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "nested/file.txt" {
		t.Fatalf("ToSlashRel() = %q, want %q", got, "nested/file.txt")
	}
}

func TestToSlashRelRoot(t *testing.T) {
	root := filepath.Join("tmp", "project")
	got, err := ToSlashRel(root, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ToSlashRel(root, root) = %q, want empty", got)
	}
}

func TestToSlashRelRejectsOutsideRoot(t *testing.T) {
	root := filepath.Join("tmp", "project")
	if _, err := ToSlashRel(root, filepath.Join("tmp", "other")); err == nil {
		t.Fatal("expected outside-root error")
	}
}
