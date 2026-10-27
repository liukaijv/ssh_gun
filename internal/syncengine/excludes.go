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
	root         string
	filterMode   string
	patterns     []string // raw exclude lines applied (defaults [+ mapping excludes])
	includeLines []string
	manual       pathMatcher
	include      pathMatcher
	disk         pathMatcher
	literal      *literalPathRules
}

func mappingFilterMode(mapping *config.SyncMapping) string {
	if mapping == nil {
		return config.FilterModeExclude
	}
	mode, err := config.NormalizeFilterMode(mapping.FilterMode)
	if err != nil {
		return config.FilterModeExclude
	}
	return mode
}

// NewExcludeMatcher builds a matcher for the given mapping.
func NewExcludeMatcher(mapping *config.SyncMapping) (*ExcludeMatcher, error) {
	mode := mappingFilterMode(mapping)
	patterns := DefaultExcludes()
	if mode != config.FilterModeInclude {
		patterns = ExcludesForMapping(mapping)
	}
	matcher := &ExcludeMatcher{
		filterMode: mode,
		patterns:   patterns,
	}
	if mapping == nil {
		matcher.manual = NewManualGitignoreMatcher(matcher.patterns)
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
	if mode == config.FilterModeInclude {
		matcher.includeLines = append([]string(nil), mapping.Includes...)
		matcher.include = NewManualGitignoreMatcher(mapping.Includes)
	}

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
	if m.filterMode == config.FilterModeInclude && !m.includeKeeps(rel, isDir) {
		return true
	}
	return diskSkip
}

// ShouldPruneDir reports whether a directory can be skipped while walking.
func (m *ExcludeMatcher) ShouldPruneDir(relPath string) bool {
	if m == nil {
		return false
	}
	rel := normalizeRel(relPath)
	if rel == "" {
		return false
	}
	if m.defaultsSkip(rel, true) {
		return true
	}
	if m.filterMode == config.FilterModeInclude {
		if !m.includeWalkable(rel) {
			return true
		}
		return m.diskShouldSkip(rel, true)
	}
	return m.ShouldSkip(rel, true)
}

func (m *ExcludeMatcher) defaultsSkip(rel string, isDir bool) bool {
	if m == nil || m.manual == nil {
		return false
	}
	detail := m.manual.MatchDetailPath(rel, isDir)
	return detail.Matched && detail.Ignored
}

func (m *ExcludeMatcher) includeKeeps(rel string, isDir bool) bool {
	if m.matchesInclude(rel, isDir) {
		return true
	}
	parent := rel
	for {
		next := path.Dir(parent)
		if next == "." || next == parent {
			break
		}
		if m.matchesInclude(next, true) {
			return true
		}
		parent = next
	}
	return isDir && m.isAncestorOfInclude(rel)
}

func (m *ExcludeMatcher) includeWalkable(dir string) bool {
	if m.includeKeeps(dir, true) {
		return true
	}
	return m.isAncestorOfInclude(dir)
}

func (m *ExcludeMatcher) matchesInclude(rel string, isDir bool) bool {
	if m.include == nil {
		return false
	}
	if m.include.MatchPath(rel, isDir) {
		return true
	}
	if isDir {
		return m.include.MatchPath(rel, false)
	}
	return m.include.MatchPath(rel, true)
}

func (m *ExcludeMatcher) isAncestorOfInclude(dir string) bool {
	dir = normalizeRel(dir)
	if dir == "" {
		return false
	}
	for _, raw := range m.includeLines {
		pat := includePattern(raw)
		if pat == "" {
			continue
		}
		if hasGlobMeta(pat) {
			slash := strings.LastIndex(pat, "/")
			if slash < 0 {
				return true
			}
			prefix := strings.Trim(pat[:slash], "/")
			if hasGlobMeta(prefix) {
				return true
			}
			prefix = normalizeRel(prefix)
			if prefix == dir || strings.HasPrefix(prefix, dir+"/") {
				return true
			}
			continue
		}
		spec := normalizeRel(strings.TrimSuffix(pat, "/"))
		if spec == dir || strings.HasPrefix(spec, dir+"/") {
			return true
		}
	}
	return false
}

func includePattern(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if strings.HasPrefix(line, "!") {
		return ""
	}
	return strings.TrimPrefix(line, "/")
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
