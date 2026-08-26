package syncengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGitignoreMatcher_RootAndNested(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gitignore"), []byte("generated.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	matcher, err := ReadGitignoreMatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	if matcher == nil {
		t.Fatal("expected matcher")
	}

	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"scratch.tmp", false, true},
		{"build", true, true},
		{"pkg/generated.go", false, true},
		{"pkg/main.go", false, false},
		{"src/main.go", false, false},
	}
	for _, tc := range cases {
		if got := matcher.MatchPath(tc.rel, tc.isDir); got != tc.want {
			t.Errorf("MatchPath(%q, %v) = %v, want %v", tc.rel, tc.isDir, got, tc.want)
		}
	}
}

func TestReadGitignoreMatcher_Negation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n!keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := ReadGitignoreMatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	if matcher.MatchPath("debug.log", false) != true {
		t.Error("*.log should be ignored")
	}
	if matcher.MatchPath("keep.log", false) != false {
		t.Error("!keep.log should not be ignored")
	}
}

func TestReadGitignoreMatcher_Empty(t *testing.T) {
	root := t.TempDir()
	matcher, err := ReadGitignoreMatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	if matcher != nil {
		t.Fatalf("got matcher %#v, want nil", matcher)
	}
}

func TestReadGitignoreMatcher_RootAnchored(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/secret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	matcher, err := ReadGitignoreMatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.MatchPath("secret.txt", false) {
		t.Error("root /secret.txt should ignore root file")
	}
	if matcher.MatchPath("sub/secret.txt", false) {
		t.Error("root /secret.txt should not ignore nested file")
	}
}
