package syncengine

import "testing"

func TestExcludeMatcher_LibraryRules(t *testing.T) {
	excludes := []string{
		"/*/Api",
		"/*/Zhiliao",
		"!/Library/Zhiliao",
		"!/Library/Api",
		"/Library/Zhiliao/Zhiliao",
		"/Library/Api/Api",
	}
	m := &ExcludeMatcher{manual: NewManualGitignoreMatcher(excludes)}

	cases := []struct {
		rel   string
		isDir bool
		skip  bool
		note  string
	}{
		{"Library/Api", true, false, "keep Api dir"},
		{"Library/Zhiliao", true, false, "keep Zhiliao dir"},
		{"Library/Api/Common", false, false, "keep under Api"},
		{"Library/Api/Controller/foo.php", false, false, "keep deep under Api"},
		{"Library/Zhiliao/foo.php", false, false, "keep under Zhiliao"},
		{"Library/Other", true, false, "keep other Library child"},
		{"Library/Api/Api", true, true, "exclude junction"},
		{"Library/Api/Api", false, true, "exclude junction as file"},
		{"Library/Zhiliao/Zhiliao", true, true, "exclude nested Zhiliao"},
		{"Library/Zhiliao/Zhiliao", false, true, "exclude nested Zhiliao file"},
		{"Foo/Api", true, true, "exclude other */Api"},
		{"Foo/Zhiliao", true, true, "exclude other */Zhiliao"},
	}
	for _, tc := range cases {
		got := m.ShouldSkip(tc.rel, tc.isDir)
		if got != tc.skip {
			d := m.manual.MatchDetailPath(tc.rel, tc.isDir)
			t.Errorf("%s: ShouldSkip(%q, %v) = %v, want %v (detail=%+v)", tc.note, tc.rel, tc.isDir, got, tc.skip, d)
		}
	}
}
