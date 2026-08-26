package syncengine

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/gitignore"
)

type literalPathRule struct {
	path    string
	ignored bool
}

// literalPathRules tracks exact-path .gitignore lines (no glob metacharacters).
// Git keeps these effective even when a later rule negates a parent directory.
type literalPathRules struct {
	rules []literalPathRule
}

func (l *literalPathRules) match(relPath string) (matched bool, ignored bool) {
	if l == nil {
		return false, false
	}
	rel := normalizeGitRel(relPath)
	for i := len(l.rules) - 1; i >= 0; i-- {
		if l.rules[i].path == rel {
			return true, l.rules[i].ignored
		}
	}
	return false, false
}

func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func appendLiteralGitignoreRules(rules []literalPathRule, relDir string, data []byte) []literalPathRule {
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = strings.TrimSpace(line[1:])
		}
		line = strings.TrimPrefix(line, "/")
		if line == "" || hasGlobMeta(line) {
			continue
		}
		pattern := line
		if relDir != "" {
			pattern = path.Join(relDir, line)
		}
		pattern = normalizeGitRel(pattern)
		if pattern == "" {
			continue
		}
		rules = append(rules, literalPathRule{path: pattern, ignored: !negate})
	}
	return rules
}

// ReadGitignoreLiteralRules loads exact-path rules from .gitignore files under root.
func ReadGitignoreLiteralRules(root string) (*literalPathRules, error) {
	if root == "" {
		return nil, nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	rules := make([]literalPathRule, 0)
	err = filepath.WalkDir(absolute, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != ".gitignore" {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		relDir, err := filepath.Rel(absolute, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}
		rules = appendLiteralGitignoreRules(rules, relDir, data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	return &literalPathRules{rules: rules}, nil
}

// pathMatcher matches slash-relative paths under a sync root.
type pathMatcher interface {
	MatchPath(relPath string, isDir bool) bool
	MatchDetailPath(relPath string, isDir bool) gitignore.MatchResult
}

type pkgsMatcher struct {
	inner *gitignore.Matcher
}

func (m *pkgsMatcher) MatchPath(relPath string, isDir bool) bool {
	if m == nil || m.inner == nil {
		return false
	}
	return m.inner.MatchPath(normalizeGitRel(relPath), isDir)
}

func (m *pkgsMatcher) MatchDetailPath(relPath string, isDir bool) gitignore.MatchResult {
	if m == nil || m.inner == nil {
		return gitignore.MatchResult{}
	}
	rel := normalizeGitRel(relPath)
	if isDir {
		return m.inner.MatchDetail(rel + "/")
	}
	return m.inner.MatchDetail(rel)
}

func normalizeGitRel(relPath string) string {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	relPath = strings.TrimPrefix(relPath, "./")
	relPath = strings.Trim(relPath, "/")
	if relPath == "." {
		return ""
	}
	return relPath
}

// NewManualGitignoreMatcher parses mapping exclude lines as root-scoped gitignore text.
func NewManualGitignoreMatcher(lines []string) pathMatcher {
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return nil
	}
	m := gitignore.New("")
	m.AddPatterns([]byte(b.String()), "")
	return &pkgsMatcher{inner: m}
}

// ReadGitignoreMatcher loads project .gitignore files under root (including nested).
// It does not load global excludes or .git/info/exclude.
// Returns nil when no .gitignore files are present.
func ReadGitignoreMatcher(root string) (pathMatcher, error) {
	if root == "" {
		return nil, nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	m := gitignore.New("")
	found := false
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != ".gitignore" {
			return nil
		}
		relDir, err := filepath.Rel(absolute, filepath.Dir(path))
		if err != nil {
			return err
		}
		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}
		m.AddFromFile(path, relDir)
		found = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &pkgsMatcher{inner: m}, nil
}
