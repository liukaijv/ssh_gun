package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestExcludeMatcher_AlwaysSkipsGit(t *testing.T) {
	manual := []string{
		".git", "node_modules", "*.log", ".idea",
		"/*.json", "/*.txt", "/Tests", "/Command",
		"/*/Api", "/*/Zhiliao",
		"!/Library/Zhiliao", "!/Library/Api",
		"/Library/Zhiliao/Zhiliao", "/Library/Api/Api",
	}
	matcher := &ExcludeMatcher{manual: NewManualGitignoreMatcher(append(DefaultExcludes(), manual...))}
	for _, rel := range []string{".git", ".git/HEAD", ".git/objects/pack/x.pack"} {
		if !matcher.ShouldSkip(rel, rel == ".git") {
			t.Errorf("ShouldSkip(%q) = false, want true", rel)
		}
		if !IsExcludedVCSPath(rel) {
			t.Errorf("IsExcludedVCSPath(%q) = false, want true", rel)
		}
	}
}

func TestExcludeMatcher_ZhiliaoConfigPHP(t *testing.T) {
	root := t.TempDir()
	gitignore := `Library/Zhiliao/Conf/config.php
*/Api
*/Zhiliao
!Library/Api
!Library/Zhiliao
`
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		t.Fatal(err)
	}

	manual := []string{
		".git", "node_modules", "*.log", ".idea",
		"/*.json", "/*.txt", "/Tests", "/Command",
		"/*/Api", "/*/Zhiliao",
		"!/Library/Zhiliao", "!/Library/Api",
		"/Library/Zhiliao/Zhiliao", "/Library/Api/Api",
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath:    root,
		UseGitignore: true,
		Excludes:     manual,
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		rel   string
		isDir bool
		skip  bool
		note  string
	}{
		{"Library/Zhiliao/Conf/config.php", false, true, "disk exact-path ignore"},
		{"Library/Zhiliao/other.php", false, false, "un-ignored under Zhiliao"},
		{"Library/Zhiliao", true, false, "keep Zhiliao dir"},
		{"Library/Zhiliao/Zhiliao", true, true, "manual junction exclude"},
		{"Foo/Zhiliao", true, true, "manual */Zhiliao"},
		{".git", true, true, "default .git exclude"},
		{".git/HEAD", false, true, "default .git child exclude"},
	}
	for _, tc := range cases {
		got := matcher.ShouldSkip(tc.rel, tc.isDir)
		if got != tc.skip {
			t.Errorf("%s: ShouldSkip(%q, %v) = %v, want %v", tc.note, tc.rel, tc.isDir, got, tc.skip)
		}
	}
}
