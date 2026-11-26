package syncengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type BrowseEntry struct {
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	IsDir   bool   `json:"isDir"`
	Skipped bool   `json:"skipped"`
	// Status is the git change kind for a leaf file in the changes tab:
	// "added", "modified", or "renamed". Directories and the files tab leave it empty.
	Status string `json:"status,omitempty"`
}

const (
	GitStatusAdded    = "added"
	GitStatusModified = "modified"
	GitStatusRenamed  = "renamed"
)

type gitChange struct {
	RelPath string
	Status  string
}

func CleanRel(rel string) (string, error) {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	if rel == "" {
		return "", fmt.Errorf("invalid relative path %q", rel)
	}
	clean := path.Clean(rel)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(filepath.FromSlash(clean)) != "" {
		return "", fmt.Errorf("invalid relative path %q", rel)
	}
	return clean, nil
}

func ensureBrowseableDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || IsLocalReparsePoint(dir) || !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}

func ListLocalDir(root, rel string, matcher *ExcludeMatcher) ([]BrowseEntry, error) {
	if err := ensureBrowseableDir(root); err != nil {
		return nil, err
	}
	absolute := root
	if rel != "" {
		clean, err := CleanRel(rel)
		if err != nil {
			return nil, err
		}
		if IsExcludedVCSPath(clean) {
			return nil, fmt.Errorf("%s is an excluded VCS path", clean)
		}
		for _, part := range strings.Split(clean, "/") {
			absolute = filepath.Join(absolute, part)
			if err := ensureBrowseableDir(absolute); err != nil {
				return nil, err
			}
		}
	}
	children, err := os.ReadDir(absolute)
	if err != nil {
		return nil, err
	}
	entries := make([]BrowseEntry, 0, len(children))
	for _, child := range children {
		if isVCSMetaDir(child.Name()) {
			continue
		}
		childAbs := filepath.Join(absolute, child.Name())
		if child.Type()&os.ModeSymlink != 0 || IsLocalReparsePoint(childAbs) {
			continue
		}
		childRel, err := ToSlashRel(root, childAbs)
		if err != nil || childRel == "" || IsExcludedVCSPath(childRel) {
			continue
		}
		isDir := child.IsDir()
		entries = append(entries, BrowseEntry{
			Name:    child.Name(),
			RelPath: childRel,
			IsDir:   isDir,
			Skipped: matcher != nil && matcher.ShouldSkip(childRel, isDir),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left != right {
			return left < right
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// ScanSubtree walks only root/rel and returns entries relative to root.
// The start directory itself is never pruned by the matcher.
func ScanSubtree(root, rel string, matcher *ExcludeMatcher) ([]FileEntry, error) {
	clean, err := CleanRel(rel)
	if err != nil {
		return nil, err
	}
	start := filepath.Join(root, filepath.FromSlash(clean))
	entries := make([]FileEntry, 0)
	err = filepath.WalkDir(start, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && isVCSMetaDir(entry.Name()) && current != start {
			return filepath.SkipDir
		}
		if isReparsePoint(entry, current) {
			if current == start {
				return fmt.Errorf("%s is a reparse point", clean)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		childRel, err := ToSlashRel(root, current)
		if err != nil {
			return err
		}
		if childRel == "" || IsExcludedVCSPath(childRel) {
			return nil
		}
		if current != start && entry.IsDir() && matcher != nil && matcher.ShouldPruneDir(childRel) {
			return filepath.SkipDir
		}
		if current != start && matcher != nil && matcher.ShouldSkip(childRel, entry.IsDir()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entries = append(entries, FileEntry{RelPath: childRel, Size: info.Size(), ModTime: info.ModTime(), IsDir: entry.IsDir()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

var ErrNotGitRepo = errors.New("not a git repository")

func ChangedFilesUnder(entries []BrowseEntry, rel string) []string {
	clean, err := CleanRel(rel)
	if err != nil {
		return nil
	}
	prefix := clean + "/"
	out := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir || entry.Skipped {
			continue
		}
		if entry.RelPath == clean || strings.HasPrefix(entry.RelPath, prefix) {
			out = append(out, entry.RelPath)
		}
	}
	return out
}

func ListGitChanges(root string, matcher *ExcludeMatcher) ([]BrowseEntry, error) {
	top, err := gitOutput(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotGitRepo
	}
	if err := ensureBrowseableDir(root); err != nil {
		return nil, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	topAbs, err := filepath.Abs(strings.TrimSpace(top))
	if err != nil {
		return nil, err
	}
	if realTop, err1 := filepath.EvalSymlinks(topAbs); err1 == nil {
		if realRoot, err2 := filepath.EvalSymlinks(rootAbs); err2 == nil {
			topAbs = realTop
			rootAbs = realRoot
		}
	}
	prefix, err := filepath.Rel(topAbs, rootAbs)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return nil, ErrNotGitRepo
	}
	prefix = filepath.ToSlash(prefix)
	raw, err := gitOutput(root, "status", "--porcelain=v1", "-uall", "-z")
	if err != nil {
		return nil, err
	}
	files := parseGitStatus(raw, prefix)
	byPath := map[string]BrowseEntry{}
	for _, change := range files {
		clean, err := CleanRel(change.RelPath)
		if err != nil || IsExcludedVCSPath(clean) {
			continue
		}
		if hasReparseAncestor(root, clean) {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(clean))
		info, err := os.Lstat(abs)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || IsLocalReparsePoint(abs) {
			continue
		}
		addBrowseFile(byPath, clean, change.Status, matcher)
	}
	entries := make([]BrowseEntry, 0, len(byPath))
	for _, entry := range byPath {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		left := strings.ToLower(entries[i].RelPath)
		right := strings.ToLower(entries[j].RelPath)
		if left != right {
			return left < right
		}
		return entries[i].RelPath < entries[j].RelPath
	})
	return entries, nil
}

func hasReparseAncestor(root, rel string) bool {
	parts := strings.Split(rel, "/")
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		cur = filepath.Join(cur, parts[i])
		info, err := os.Lstat(cur)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || IsLocalReparsePoint(cur) {
			return true
		}
	}
	return false
}

func addBrowseFile(byPath map[string]BrowseEntry, rel, status string, matcher *ExcludeMatcher) {
	parts := strings.Split(rel, "/")
	acc := ""
	for i, part := range parts {
		if acc == "" {
			acc = part
		} else {
			acc = acc + "/" + part
		}
		last := i == len(parts)-1
		if _, ok := byPath[acc]; ok && !last {
			continue
		}
		entry := BrowseEntry{
			Name:    part,
			RelPath: acc,
			IsDir:   !last,
			Skipped: matcher != nil && matcher.ShouldSkip(acc, !last),
		}
		if last {
			entry.Status = status
		}
		byPath[acc] = entry
	}
}

func gitChangeKind(xy string) string {
	if xy == "??" || xy[0] == 'A' || xy[1] == 'A' {
		return GitStatusAdded
	}
	if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
		return GitStatusRenamed
	}
	return GitStatusModified
}

func parseGitStatus(raw, prefix string) []gitChange {
	parts := strings.Split(raw, "\x00")
	out := make([]gitChange, 0)
	for i := 0; i < len(parts); i++ {
		record := parts[i]
		if len(record) < 3 {
			continue
		}
		xy := record[:2]
		filePath := record[3:]
		if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
			i++
		}
		if xy[1] == 'D' || (xy[0] == 'D' && xy[1] == ' ') {
			continue
		}
		mapped := mapGitPath(filePath, prefix)
		if mapped != "" {
			out = append(out, gitChange{RelPath: mapped, Status: gitChangeKind(xy)})
		}
	}
	return out
}

func mapGitPath(repoPath, prefix string) string {
	repoPath = filepath.ToSlash(strings.TrimSpace(repoPath))
	if prefix == "" || prefix == "." {
		return repoPath
	}
	if repoPath == prefix {
		return ""
	}
	head := prefix + "/"
	if !strings.HasPrefix(repoPath, head) {
		return ""
	}
	return strings.TrimPrefix(repoPath, head)
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	configureHiddenCmd(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
