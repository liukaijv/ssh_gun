package rsyncbin

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestCygpath(t *testing.T) {
	tests := map[string]string{
		`D:\foo`:             `/cygdrive/d/foo`,
		`C:\Program Files\x`: `/cygdrive/c/Program Files/x`,
		`d:/foo/bar/`:        `/cygdrive/d/foo/bar/`,
		`\\server\share\x`:   `//server/share/x`,
		`relative\path`:      `relative/path`,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := Cygpath(input); got != want {
				t.Fatalf("Cygpath(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestExtractCopiesTreeAndUsesVersionMarker(t *testing.T) {
	source := fstest.MapFS{
		"bundle/bin/rsync.exe": &fstest.MapFile{Data: []byte("first"), Mode: 0o755},
		"bundle/etc/config":    &fstest.MapFile{Data: []byte("config"), Mode: 0o644},
	}
	localAppData := t.TempDir()

	runtimeDir, err := Extract(source, "bundle", localAppData)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(localAppData, "ssh_gun", "runtime", "cwrsync-"+Version)
	if runtimeDir != wantDir {
		t.Fatalf("runtime dir = %q, want %q", runtimeDir, wantDir)
	}
	assertFileContents(t, filepath.Join(runtimeDir, "bin", "rsync.exe"), "first")
	assertFileContents(t, filepath.Join(runtimeDir, "etc", "config"), "config")
	assertFileContents(t, filepath.Join(runtimeDir, ".version"), Version)

	source["bundle/bin/rsync.exe"] = &fstest.MapFile{Data: []byte("second"), Mode: fs.FileMode(0o755)}
	if _, err := Extract(source, "bundle", localAppData); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, filepath.Join(runtimeDir, "bin", "rsync.exe"), "first")
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
