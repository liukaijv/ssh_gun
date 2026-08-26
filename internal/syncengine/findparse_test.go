package syncengine

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParseFindPrintf(t *testing.T) {
	output := "d\t\t4096\t1588334400.0000000000\n" +
		"d\tsub\t4096\t1588334401.2500000000\n" +
		"f\tsub/file.txt\t11\t1588334402.0000000000\n"
	want := []FileEntry{
		{RelPath: "", Size: 4096, ModTime: time.Unix(1588334400, 0), IsDir: true},
		{RelPath: "sub", Size: 4096, ModTime: time.Unix(1588334401, 250000000), IsDir: true},
		{RelPath: "sub/file.txt", Size: 11, ModTime: time.Unix(1588334402, 0)},
	}
	got, err := ParseFindPrintf(output)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("entries mismatch (-want +got):\n%s", diff)
	}
}

func TestParseFindPrintfRejectsMalformedLine(t *testing.T) {
	if _, err := ParseFindPrintf("f\tmissing-fields\n"); err == nil {
		t.Fatal("expected malformed line error")
	}
}

func TestParseFindPrintfRejectsUnknownType(t *testing.T) {
	if _, err := ParseFindPrintf("l\tlink\t1\t1.0\n"); err == nil {
		t.Fatal("expected unknown type error")
	}
}
