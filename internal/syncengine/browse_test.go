package syncengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ssh_gun/internal/config"
)

func TestCleanRelRejectsEscape(t *testing.T) {
	if _, err := CleanRel(""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := CleanRel("."); err == nil {
		t.Fatal("expected error")
	}
	if _, err := CleanRel("../x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := CleanRel("/abs"); err == nil {
		t.Fatal("expected error")
	}
	if runtime.GOOS == "windows" {
		if _, err := CleanRel(`C:\abs`); err == nil {
			t.Fatal("expected error for Windows drive path")
		}
	}
	got, err := CleanRel(`a\b`)
	if err != nil || got != "a/b" {
		t.Fatalf("CleanRel = %q, %v", got, err)
	}
}

func TestListLocalDirOneLevelGrayAndHidden(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "a")
	mustWrite(t, filepath.Join(root, "skip.txt"), "b")
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "sub", "nested.txt"), "c")
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")

	symlinkOK := true
	if err := os.Symlink(filepath.Join(root, "keep.txt"), filepath.Join(root, "link.txt")); err != nil {
		symlinkOK = false
		t.Logf("symlink unsupported, skipping link.txt assertions: %v", err)
	}

	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "inner", "secret.txt"), "s")
	dirSymlinkOK := false
	if symlinkOK {
		if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err == nil {
			dirSymlinkOK = true
		}
	}

	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		LocalPath: root,
		Excludes:  []string{"skip.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ListLocalDir(root, "", matcher)
	if err != nil {
		t.Fatal(err)
	}
	if symlinkOK {
		for _, entry := range entries {
			if entry.Name == "link.txt" || entry.RelPath == "link.txt" {
				t.Fatalf("expected link.txt to be hidden, got %+v", entries)
			}
		}
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v, want sub, keep.txt, skip.txt", entries)
	}
	if !entries[0].IsDir || entries[0].RelPath != "sub" || entries[0].Name != "sub" || entries[0].Skipped {
		t.Fatalf("first = %+v", entries[0])
	}
	if entries[1].IsDir || entries[1].RelPath != "keep.txt" || entries[1].Name != "keep.txt" || entries[1].Skipped {
		t.Fatalf("keep file = %+v", entries[1])
	}
	if entries[1].Size != 1 || entries[1].ModTime.IsZero() {
		t.Fatalf("keep file meta = %+v", entries[1])
	}
	if entries[2].IsDir || entries[2].RelPath != "skip.txt" || entries[2].Name != "skip.txt" || !entries[2].Skipped {
		t.Fatalf("gray file = %+v", entries[2])
	}
	if entries[0].ModTime.IsZero() {
		t.Fatalf("dir meta = %+v", entries[0])
	}
	nested, err := ListLocalDir(root, "sub", matcher)
	if err != nil {
		t.Fatal(err)
	}
	if len(nested) != 1 || nested[0].RelPath != "sub/nested.txt" || nested[0].Size != 1 || nested[0].ModTime.IsZero() {
		t.Fatalf("nested = %+v", nested)
	}
	if _, err := ListLocalDir(root, "keep.txt", matcher); err == nil {
		t.Fatal("file path must fail")
	}
	if _, err := ListLocalDir(root, ".git", matcher); err == nil {
		t.Fatal(".git path must fail")
	}
	if dirSymlinkOK {
		if _, err := ListLocalDir(root, "linkdir/inner", matcher); err == nil {
			t.Fatal("symlink parent path must fail")
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListGitChangesFiltersStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	gitRun(t, root, "init")
	mustWrite(t, filepath.Join(root, "tracked.txt"), "one")
	mustWrite(t, filepath.Join(root, "gone.txt"), "old")
	gitRun(t, root, "add", "tracked.txt", "gone.txt")
	gitRun(t, root, "commit", "-m", "init")
	mustWrite(t, filepath.Join(root, "tracked.txt"), "two")
	mustWrite(t, filepath.Join(root, "fresh.txt"), "new")
	mustWrite(t, filepath.Join(root, "secret.log"), "nope")
	mustWrite(t, filepath.Join(root, ".gitignore"), "secret.log\n")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "tracked.txt"), filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatal(err)
	}

	matcher, err := NewExcludeMatcher(&config.SyncMapping{LocalPath: root, Excludes: []string{"fresh.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ListGitChanges(root, matcher)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]BrowseEntry{}
	for _, entry := range entries {
		got[entry.RelPath] = entry
	}
	if _, ok := got["gone.txt"]; ok {
		t.Fatal("deleted file was listed")
	}
	if _, ok := got["secret.log"]; ok {
		t.Fatal("gitignored untracked file was listed")
	}
	if _, ok := got["tracked.txt"]; ok {
		t.Fatal("rename old path was listed")
	}
	file, ok := got["renamed.txt"]
	if !ok || file.IsDir {
		t.Fatalf("renamed file missing: %+v", entries)
	}
	if file.Status != GitStatusRenamed && file.Status != GitStatusAdded {
		// Unstaged rename shows as deleted old + untracked new ("??"), or as "R " when staged.
		t.Fatalf("renamed.txt status = %q, want renamed or added", file.Status)
	}
	fresh, ok := got["fresh.txt"]
	if !ok || !fresh.Skipped || fresh.Status != GitStatusAdded {
		t.Fatalf("fresh.txt = %+v", fresh)
	}
	if fresh.Size != 3 || fresh.ModTime.IsZero() {
		t.Fatalf("fresh.txt meta = %+v", fresh)
	}
	plain := t.TempDir()
	if _, err := ListGitChanges(plain, matcher); !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseGitStatusKinds(t *testing.T) {
	got := parseGitStatus(
		"?? untracked.txt\x00A  staged_new.txt\x00AM staged_and_edit.txt\x00 M modified.txt\x00"+
			"R  new.txt\x00old.txt\x00C  copy.txt\x00orig.txt\x00RM ren_and_edit.txt\x00was.txt\x00"+
			" D del.txt\x00D  staged_del.txt\x00",
		"",
	)
	want := map[string]string{
		"untracked.txt":       GitStatusAdded,
		"staged_new.txt":      GitStatusAdded,
		"staged_and_edit.txt": GitStatusAdded,
		"modified.txt":        GitStatusModified,
		"new.txt":             GitStatusRenamed,
		"copy.txt":            GitStatusRenamed,
		"ren_and_edit.txt":    GitStatusRenamed,
	}
	if len(got) != len(want) {
		t.Fatalf("parseGitStatus = %+v, want %d entries", got, len(want))
	}
	for _, change := range got {
		if want[change.RelPath] != change.Status {
			t.Fatalf("%s status = %q, want %q (all=%+v)", change.RelPath, change.Status, want[change.RelPath], got)
		}
		delete(want, change.RelPath)
	}
	if len(want) != 0 {
		t.Fatalf("missing paths: %+v", want)
	}
}

func TestListGitChangesStagedRenameAndSubdir(t *testing.T) {
	gotParsed := parseGitStatus("R  new.txt\x00old.txt\x00C  copy.txt\x00orig.txt\x00 D del.txt\x00D  staged_del.txt\x00", "")
	if len(gotParsed) != 2 || gotParsed[0].RelPath != "new.txt" || gotParsed[1].RelPath != "copy.txt" {
		t.Fatalf("parseGitStatus = %+v, want [new.txt copy.txt]", gotParsed)
	}
	if gotParsed[0].Status != GitStatusRenamed || gotParsed[1].Status != GitStatusRenamed {
		t.Fatalf("rename/copy status = %+v", gotParsed)
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	gitRun(t, root, "init")
	mustWrite(t, filepath.Join(root, "orig.txt"), "same content\n")
	mustWrite(t, filepath.Join(root, "cached_gone.log"), "secret\n")
	mustWrite(t, filepath.Join(root, ".gitignore"), "*.log\n")
	gitRun(t, root, "add", "-f", "orig.txt", "cached_gone.log", ".gitignore")
	gitRun(t, root, "commit", "-m", "init")

	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "mv", "orig.txt", "sub/moved.txt")
	gitRun(t, root, "rm", "--cached", "cached_gone.log")
	mustWrite(t, filepath.Join(root, "root_only.txt"), "root\n")

	entries, err := ListGitChanges(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]BrowseEntry{}
	for _, entry := range entries {
		got[entry.RelPath] = entry
	}
	if _, ok := got["orig.txt"]; ok {
		t.Fatalf("staged rename old path listed: %+v", entries)
	}
	if _, ok := got["cached_gone.log"]; ok {
		t.Fatalf("cached-deleted gitignored file listed: %+v", entries)
	}
	subDir, ok := got["sub"]
	if !ok || !subDir.IsDir || subDir.Name != "sub" {
		t.Fatalf("missing synthetic dir sub: %+v", entries)
	}
	moved, ok := got["sub/moved.txt"]
	if !ok || moved.IsDir || moved.Name != "moved.txt" || moved.Status != GitStatusRenamed {
		t.Fatalf("missing staged rename new path sub/moved.txt: %+v", entries)
	}
	if subDir.Status != "" {
		t.Fatalf("synthetic dir must have empty status: %+v", subDir)
	}
	rootOnly, ok := got["root_only.txt"]
	if !ok || rootOnly.Status != GitStatusAdded {
		t.Fatalf("root_only.txt = %+v", rootOnly)
	}
	if !entries[0].IsDir {
		t.Fatalf("expected directories sorted first, got %+v", entries)
	}

	subEntries, err := ListGitChanges(filepath.Join(root, "sub"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(subEntries) != 1 || subEntries[0].RelPath != "moved.txt" || subEntries[0].IsDir {
		t.Fatalf("subdir ListGitChanges = %+v, want [moved.txt]", subEntries)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestChangedFilesUnderDirectory(t *testing.T) {
	entries := []BrowseEntry{
		{RelPath: "src", IsDir: true},
		{RelPath: "src/a.txt"},
		{RelPath: "src/skip.txt", Skipped: true},
		{RelPath: "other.txt"},
	}
	got := ChangedFilesUnder(entries, "src")
	if len(got) != 1 || got[0] != "src/a.txt" {
		t.Fatalf("got %#v", got)
	}
}
