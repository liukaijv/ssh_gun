package syncengine

import (
	"testing"

	"ssh_gun/internal/config"
)

func TestExcludeMatcher_UserApiProjectRules(t *testing.T) {
	excludes := []string{
		".git", "node_modules", "*.log", ".idea",
		"/*.json", "/*.text", "/Tests", "/Command",
		"/*/Api", "/*/Zhiliao",
		"!/Library/Zhiliao", "!/Library/Api",
	}
	matcher := &ExcludeMatcher{manual: NewManualGitignoreMatcher(excludes)}

	cases := []struct {
		rel   string
		isDir bool
		skip  bool
	}{
		{"Library/Api", true, false},
		{"Library/Api/Api", false, false},
		{"Library/Api/Api", true, false},
		{"Library/Zhiliao", true, false},
		{"Command", true, true},
		{"Tests", true, true},
		{"Foo/Api", true, true},
		{"config.json", false, true},
	}
	for _, tc := range cases {
		got := matcher.ShouldSkip(tc.rel, tc.isDir)
		if got != tc.skip {
			detail := matcher.manual.MatchDetailPath(tc.rel, tc.isDir)
			t.Errorf("ShouldSkip(%q, %v) = %v, want %v (detail=%+v)", tc.rel, tc.isDir, got, tc.skip, detail)
		}
	}
}

func TestExcludeMatcher_UserRulesWithMapping(t *testing.T) {
	matcher, err := NewExcludeMatcher(&config.SyncMapping{
		Excludes: []string{
			".git", "node_modules", "*.log", ".idea",
			"/*.json", "/*.text", "/Tests", "/Command",
			"/*/Api", "/*/Zhiliao",
			"!/Library/Zhiliao", "!/Library/Api",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matcher.ShouldSkip("Library/Api/Api", false) {
		t.Log("Library/Api/Api is excluded")
	} else {
		t.Log("Library/Api/Api is NOT excluded - will be synced")
	}
}
