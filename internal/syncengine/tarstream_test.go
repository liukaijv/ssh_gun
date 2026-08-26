package syncengine

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
	"time"
)

func TestWriteTarRoundTrip(t *testing.T) {
	modTime := time.Unix(1700000000, 0)
	files := []TarFile{
		{RelPath: "dir", ModTime: modTime, Mode: 0o755, IsDir: true},
		{RelPath: "dir/hello.txt", Content: []byte("hello"), ModTime: modTime, Mode: 0o640},
	}
	var buf bytes.Buffer
	if err := WriteTar(&buf, files); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&buf)
	dir, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if dir.Name != "dir/" || dir.Typeflag != tar.TypeDir || dir.Mode != 0o755 {
		t.Fatalf("unexpected directory header: %#v", dir)
	}

	file, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "dir/hello.txt" || file.Mode != 0o640 || string(content) != "hello" {
		t.Fatalf("unexpected file: header=%#v content=%q", file, content)
	}
	if !file.ModTime.Equal(modTime) {
		t.Fatalf("modtime = %v, want %v", file.ModTime, modTime)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("final Next error = %v, want EOF", err)
	}
}

func TestWriteTarRejectsUnsafePath(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTar(&buf, []TarFile{{RelPath: "../escape", Content: []byte("bad")}})
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
}
