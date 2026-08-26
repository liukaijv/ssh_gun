//go:build windows

package rshbridge

import (
	"io"
	"os"
	"testing"
)

func TestDuplicateInputReadsWithoutOwningOriginalHandle(t *testing.T) {
	original, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	defer writer.Close()

	duplicate, err := duplicateInput(original)
	if err != nil {
		t.Fatal(err)
	}
	defer duplicate.Close()
	if duplicate.Fd() == original.Fd() {
		t.Fatal("duplicate reused the standard input handle")
	}

	if _, err := writer.Write([]byte("rsync")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len("rsync"))
	if _, err := io.ReadFull(duplicate, buffer); err != nil {
		t.Fatal(err)
	}
	if got := string(buffer); got != "rsync" {
		t.Fatalf("read %q, want rsync", got)
	}
	if err := duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := original.Stat(); err != nil {
		t.Fatalf("closing duplicate closed original input: %v", err)
	}
}
