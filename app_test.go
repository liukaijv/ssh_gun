package main

import (
	"os"
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestOpenConfigDirectoryCreatesAndOpensParent(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ssh_gun", "config.toml")
	var opened string

	err := openConfigDirectory(configPath, func(path string) error {
		opened = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Dir(configPath)
	if opened != want {
		t.Fatalf("opened %q, want %q", opened, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("config directory was not created: %v", err)
	}
}

func TestMatcherFingerprintStableAndSensitive(t *testing.T) {
	base := config.SyncMapping{
		ID:           "m1",
		LocalPath:    `D:\proj`,
		FilterMode:   "exclude",
		UseGitignore: true,
		Excludes:     []string{"tmp/", "*.log"},
		Includes:     []string{"src/"},
	}
	a := matcherFingerprint(&base)
	b := matcherFingerprint(&base)
	if a == "" || a != b {
		t.Fatalf("fingerprint unstable: %q vs %q", a, b)
	}
	changed := base
	changed.UseGitignore = false
	if matcherFingerprint(&changed) == a {
		t.Fatal("fingerprint should change when UseGitignore flips")
	}
	changed = base
	changed.Excludes = []string{"tmp/", "*.bak"}
	if matcherFingerprint(&changed) == a {
		t.Fatal("fingerprint should change when Excludes change")
	}
}

func TestExcludeMatcherForCachesAndInvalidates(t *testing.T) {
	root := t.TempDir()
	app := NewApp()
	m := config.SyncMapping{
		ID:           "cache-m",
		LocalPath:    root,
		UseGitignore: false,
	}
	first, err := app.excludeMatcherFor(&m)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.excludeMatcherFor(&m)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected cached ExcludeMatcher pointer reuse")
	}
	app.invalidateBrowseMatcher(m.ID)
	third, err := app.excludeMatcherFor(&m)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("expected new matcher after invalidate")
	}
	m.UseGitignore = true
	fourth, err := app.excludeMatcherFor(&m)
	if err != nil {
		t.Fatal(err)
	}
	if fourth == third {
		t.Fatal("expected new matcher when fingerprint changes")
	}
}
