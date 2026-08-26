package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestShouldMatch(t *testing.T) {
	patterns := []string{".git", "node_modules", "build/**", "*.log"}
	tests := map[string]bool{
		".git":                    true,
		".git/config":             true,
		"src/node_modules/pkg.js": true,
		"build/app/main.js":       true,
		"debug.log":               true,
		"logs/debug.log":          true,
		"src/main.go":             false,
	}
	for path, want := range tests {
		if got := ShouldMatch(path, patterns); got != want {
			t.Errorf("ShouldMatch(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestShouldSkipDir(t *testing.T) {
	patterns := []string{".git", "node_modules", "build/**", "*.log"}
	for _, path := range []string{".git", "src/node_modules", "build", "build/generated"} {
		if !ShouldSkipDir(path, patterns) {
			t.Errorf("ShouldSkipDir(%q) = false, want true", path)
		}
	}
	if ShouldSkipDir("src", patterns) {
		t.Error("ShouldSkipDir(\"src\") = true, want false")
	}
}

func TestDefaultExcludes(t *testing.T) {
	got := DefaultExcludes()
	want := []string{".git/", "*~", "*.swp"}
	if len(got) != len(want) {
		t.Fatalf("DefaultExcludes() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DefaultExcludes()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExcludesForMapping(t *testing.T) {
	mapping := &config.SyncMapping{Excludes: []string{"vendor", "*.tmp"}}
	got := ExcludesForMapping(mapping)

	for _, path := range []string{"vendor/pkg.go", "scratch.tmp", ".git/config"} {
		if !ShouldMatch(path, got) {
			t.Errorf("ExcludesForMapping result should match %q", path)
		}
	}
	if len(got) != len(DefaultExcludes())+len(mapping.Excludes) {
		t.Fatalf("len(ExcludesForMapping) = %d, want %d", len(got), len(DefaultExcludes())+len(mapping.Excludes))
	}
}

func TestExcludeMatcher_ManualAndGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("dist/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		Excludes:     []string{"vendor/"},
		UseGitignore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip("vendor/lib.go", false) {
		t.Error("manual exclude should skip vendor")
	}
	if !matcher.ShouldSkip("dist", true) {
		t.Error("gitignore should skip dist/")
	}
	if !matcher.ShouldSkip("app.log", false) {
		t.Error("gitignore should skip *.log")
	}
	if matcher.ShouldSkip("src/main.go", false) {
		t.Error("src/main.go should not be skipped")
	}
}

func TestExcludeMatcher_ManualNegationOverridesDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		Excludes:     []string{"!keep.log"},
		UseGitignore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("keep.log", false) {
		t.Error("manual !keep.log should override disk *.log")
	}
	if !matcher.ShouldSkip("other.log", false) {
		t.Error("other.log should still be ignored by disk")
	}
}

func TestExcludeMatcher_ManualIgnoreOverridesDiskNegation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n!keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		Excludes:     []string{"*.log"},
		UseGitignore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip("keep.log", false) {
		t.Error("manual *.log should override disk !keep.log")
	}
}

func TestExcludeMatcher_GitignoreDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		UseGitignore: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("secret.txt", false) {
		t.Error("gitignore rules must not apply when UseGitignore is false")
	}
}

func TestExcludeMatcher_RootAnchoredManual(t *testing.T) {
	root := t.TempDir()
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath: root,
		Excludes:  []string{"/only-root.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip("only-root.txt", false) {
		t.Error("should skip root file")
	}
	if matcher.ShouldSkip("sub/only-root.txt", false) {
		t.Error("should not skip nested file for /only-root.txt")
	}
}
