package syncengine

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestDiffMtimeSize(t *testing.T) {
	base := time.Unix(1000, 0)
	local := []FileEntry{
		{RelPath: "newdir", IsDir: true},
		{RelPath: "same.txt", Size: 4, ModTime: base},
		{RelPath: "changed.txt", Size: 8, ModTime: base},
		{RelPath: "new.txt", Size: 3, ModTime: base},
	}
	remote := []FileEntry{
		{RelPath: "same.txt", Size: 4, ModTime: base.Add(500 * time.Millisecond)},
		{RelPath: "changed.txt", Size: 7, ModTime: base},
		{RelPath: "old.txt", Size: 2, ModTime: base},
	}

	want := Changes{
		Upload: []FileEntry{local[2], local[3]},
		Mkdir:  []FileEntry{local[0]},
		Delete: []FileEntry{remote[2]},
	}
	got := Diff(local, remote, "mtime-size", time.Second)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Diff mismatch (-want +got):\n%s", diff)
	}
}

func TestDiffChecksum(t *testing.T) {
	local := []FileEntry{
		{RelPath: "same", Size: 1, Hash: "abc"},
		{RelPath: "changed", Size: 1, Hash: "new"},
	}
	remote := []FileEntry{
		{RelPath: "same", Size: 99, Hash: "abc"},
		{RelPath: "changed", Size: 1, Hash: "old"},
	}
	got := Diff(local, remote, "checksum", 0)
	if diff := cmp.Diff([]FileEntry{local[1]}, got.Upload); diff != "" {
		t.Fatalf("Upload mismatch (-want +got):\n%s", diff)
	}
}

func TestDiffReplacesEntryWithDifferentKind(t *testing.T) {
	local := []FileEntry{{RelPath: "item", IsDir: true}}
	remote := []FileEntry{{RelPath: "item", Size: 10}}
	got := Diff(local, remote, "mtime-size", 0)
	if len(got.Delete) != 1 || len(got.Mkdir) != 1 {
		t.Fatalf("got %#v, want delete followed by mkdir", got)
	}
}
