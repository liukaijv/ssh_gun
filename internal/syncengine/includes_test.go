package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestIncludeMatcher_DirectoryIncludesDescendants(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"src/"},
		Excludes:   []string{"src/skip.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("src", true) {
		t.Fatal("included directory should not skip")
	}
	if matcher.ShouldSkip("src/main.go", false) {
		t.Fatal("descendant of included directory should not skip")
	}
	if matcher.ShouldSkip("src/pkg/util.go", false) {
		t.Fatal("nested descendant should not skip")
	}
	if !matcher.ShouldSkip("lib/main.go", false) {
		t.Fatal("sibling tree should skip")
	}
	if matcher.ShouldSkip("src/skip.go", false) {
		t.Fatal("exclude text must not apply in include mode")
	}
}

func TestIncludeMatcher_FileAndAncestors(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"docs/guide.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("docs", true) {
		t.Fatal("ancestor directory should not skip (mkdir)")
	}
	if matcher.ShouldSkip("docs/guide.md", false) {
		t.Fatal("included file should not skip")
	}
	if !matcher.ShouldSkip("docs/other.md", false) {
		t.Fatal("sibling file should skip")
	}
	if !matcher.ShouldSkip("README.md", false) {
		t.Fatal("unrelated file should skip")
	}
	if !matcher.ShouldPruneDir("lib") {
		t.Fatal("unrelated directory should prune")
	}
	if matcher.ShouldPruneDir("docs") {
		t.Fatal("ancestor of include should not prune")
	}
}

func TestIncludeMatcher_DefaultExcludesStillApply(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"src/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip(".git", true) {
		t.Fatal(".git should skip")
	}
	if !matcher.ShouldSkip("src/foo.swp", false) {
		t.Fatal("default *.swp should skip inside include tree")
	}
}

func TestIncludeMatcher_GlobDoesNotPruneUnrelatedTrees(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"*.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("README.md", false) {
		t.Fatal("root markdown should not skip")
	}
	if matcher.ShouldSkip("docs/guide.md", false) {
		t.Fatal("nested markdown should not skip")
	}
	if !matcher.ShouldSkip("src/main.go", false) {
		t.Fatal("non-matching file should skip")
	}
	if matcher.ShouldPruneDir("docs") {
		t.Fatal("glob include must still walk directories")
	}
}

func TestIncludeMatcher_EmptyIncludesSkipAll(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip("src/main.go", false) {
		t.Fatal("empty include should skip files")
	}
	if !matcher.ShouldPruneDir("src") {
		t.Fatal("empty include should prune directories")
	}
}

func TestIncludeMatcher_UseGitignoreInsideInclude(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "secret.txt"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "ok.go"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		FilterMode:   config.FilterModeInclude,
		Includes:     []string{"src/"},
		UseGitignore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldSkip("src/secret.txt", false) {
		t.Fatal("gitignore should still skip inside include tree")
	}
	if matcher.ShouldSkip("src/ok.go", false) {
		t.Fatal("non-gitignored included file should not skip")
	}
}

func TestIncludeMatcher_ScopedDeleteSemantics(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"src/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("src/extra.go", false) {
		t.Fatal("extra file inside include tree should be visible for delete-extra")
	}
	if !matcher.ShouldSkip("lib/only-remote.go", false) {
		t.Fatal("file outside include tree should be invisible so delete-extra cannot touch it")
	}
}

func TestScanLocal_IncludeModeAddsAncestors(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/guide.md")
	write("docs/other.md")
	write("lib/a.go")
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:  root,
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"docs/guide.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLocal(root, matcher)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.RelPath] = true
	}
	if !seen["docs"] || !seen["docs/guide.md"] {
		t.Fatalf("entries = %#v, want docs and docs/guide.md", entries)
	}
	if seen["docs/other.md"] || seen["lib"] || seen["lib/a.go"] {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}
