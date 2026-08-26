package syncengine

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/git-pkgs/gitignore"

	"ssh_gun/internal/config"
)

// DefaultExcludes returns built-in exclude patterns always applied on top of
// mapping.Excludes. .git/ is included so a missing or mistyped manual rule
// cannot upload a repository directory.
func DefaultExcludes() []string {
	return []string{".git/", "*~", "*.swp"}
}

// ExcludesForMapping returns default excludes followed by mapping-specific ones.
func ExcludesForMapping(mapping *config.SyncMapping) []string {
	out := append([]string(nil), DefaultExcludes()...)
	if mapping == nil {
		return out
	}
	return append(out, mapping.Excludes...)
}

// isVCSMetaDir reports directory names that must never be walked or uploaded.
func isVCSMetaDir(name string) bool {
	return name == ".git"
}

// IsExcludedVCSPath reports paths under a VCS metadata directory (e.g. .git/...).
func IsExcludedVCSPath(relPath string) bool {
	rel := normalizeRel(relPath)
	if rel == "" {
		return false
	}
	if isVCSMetaDir(rel) {
		return true
	}
	for _, part := range strings.Split(rel, "/") {
		if isVCSMetaDir(part) {
			return true
		}
	}
	return false
}

// ExcludeMatcher combines manual gitignore text with optional on-disk .gitignore rules.
// Manual ignore rules win immediately. Manual directory un-ignore (!dir) still allows
// disk rules (including exact-path .gitignore lines) to exclude nested files.
type ExcludeMatcher struct {
	root     string
	patterns []string // raw lines (for rsync / callers)
	manual   pathMatcher
	disk     pathMatcher
	literal  *literalPathRules
}

// NewExcludeMatcher builds a matcher for the given mapping.
func NewExcludeMatcher(mapping *config.SyncMapping) (*ExcludeMatcher, error) {
	matcher := &ExcludeMatcher{
		patterns: ExcludesForMapping(mapping),
	}
	if mapping == nil {
		return matcher, nil
	}
	if mapping.LocalPath != "" {
		if absolute, err := filepath.Abs(mapping.LocalPath); err == nil {
			matcher.root = filepath.Clean(absolute)
		} else {
			matcher.root = mapping.LocalPath
		}
	}
	matcher.manual = NewManualGitignoreMatcher(matcher.patterns)

	if !mapping.UseGitignore || mapping.LocalPath == "" {
		return matcher, nil
	}
	if _, err := os.Stat(matcher.root); err != nil {
		if os.IsNotExist(err) {
			return matcher, nil
		}
		return nil, err
	}
	disk, err := ReadGitignoreMatcher(matcher.root)
	if err != nil {
		return nil, err
	}
	matcher.disk = disk
	literal, err := ReadGitignoreLiteralRules(matcher.root)
	if err != nil {
		return nil, err
	}
	matcher.literal = literal
	return matcher, nil
}

// Patterns returns the raw manual exclude lines.
func (m *ExcludeMatcher) Patterns() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.patterns...)
}

// ShouldSkip reports whether relPath should be excluded from sync.
func (m *ExcludeMatcher) ShouldSkip(relPath string, isDir bool) bool {
	if m == nil {
		return false
	}
	rel := normalizeRel(relPath)
	if rel == "" {
		return false
	}

	var manualDetail gitignore.MatchResult
	if m.manual != nil {
		manualDetail = m.manual.MatchDetailPath(rel, isDir)
		if manualDetail.Matched && manualDetail.Ignored {
			return true
		}
	}

	diskSkip := m.diskShouldSkip(rel, isDir)

	if manualDetail.Matched && manualDetail.Negate && !manualDetail.Ignored {
		if negationOverridesPath(manualDetail.Pattern, rel) {
			return false
		}
		return diskSkip
	}
	if manualDetail.Matched {
		return manualDetail.Ignored
	}
	return diskSkip
}

func (m *ExcludeMatcher) diskShouldSkip(rel string, isDir bool) bool {
	if m.literal != nil {
		if matched, ignored := m.literal.match(rel); matched {
			return ignored
		}
	}
	if m.disk == nil {
		return false
	}
	return m.disk.MatchPath(rel, isDir)
}

func negationOverridesPath(pattern, rel string) bool {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.TrimPrefix(pattern, "!")
	pattern = strings.TrimPrefix(pattern, "/")
	rel = normalizeRel(rel)
	if pattern == rel {
		return true
	}
	if !strings.Contains(pattern, "/") && path.Base(rel) == pattern {
		return true
	}
	return false
}

// ShouldSkipUnknown reports whether relPath should be skipped when directory
// status is unknown (matches if either file or directory rules apply).
func (m *ExcludeMatcher) ShouldSkipUnknown(relPath string) bool {
	return m.ShouldSkip(relPath, false) || m.ShouldSkip(relPath, true)
}

// ShouldMatch reports whether relPath matches any of the simple glob patterns.
// Kept for tests and legacy callers; sync paths use ExcludeMatcher instead.
func ShouldMatch(relPath string, patterns []string) bool {
	relPath = normalizeRel(relPath)
	if relPath == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = normalizeRel(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(pattern, "/") {
			if matched, _ := doublestar.Match(pattern, relPath); matched {
				return true
			}
			continue
		}
		for _, part := range strings.Split(relPath, "/") {
			if matched, _ := doublestar.Match(pattern, part); matched {
				return true
			}
		}
	}
	return false
}

func ShouldSkipDir(relPath string, patterns []string) bool {
	return ShouldMatch(relPath, patterns)
}

func normalizeRel(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimPrefix(value, "./")
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return ""
	}
	return path.Clean(value)
}
