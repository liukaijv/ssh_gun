package syncengine

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ssh_gun/internal/config"
)

func TestScanLocal(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("keep/file.txt", "hello")
	writeFile("build/generated.bin", "skip")
	writeFile(".git/config", "skip")
	writeFile("debug.log", "skip")

	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath: root,
		Excludes:  []string{".git", "build/", "*.log"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLocal(root, matcher)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })

	if len(entries) != 2 {
		t.Fatalf("got %#v, want keep directory and file only", entries)
	}
	if entries[0].RelPath != "keep" || !entries[0].IsDir {
		t.Fatalf("unexpected directory entry: %#v", entries[0])
	}
	if entries[1].RelPath != "keep/file.txt" || entries[1].IsDir || entries[1].Size != 5 {
		t.Fatalf("unexpected file entry: %#v", entries[1])
	}
	if filepath.Separator == '\\' && entries[1].RelPath != "keep/file.txt" {
		t.Fatalf("path is not slash-normalized: %q", entries[1].RelPath)
	}
}

func TestScanLocal_UsesGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{LocalPath: root, UseGitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLocal(root, matcher)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.RelPath == "ignored.txt" {
			t.Fatalf("gitignore path was scanned: %#v", entries)
		}
	}
}

func TestScanLocalMissingRoot(t *testing.T) {
	if _, err := ScanLocal(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Fatal("expected missing root error")
	}
}
