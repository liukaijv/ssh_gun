package config_test

import (
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizeTheme(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "system", true},
		{"system", "system", true},
		{"light", "light", true},
		{"dark", "dark", true},
		{"blue", "", false},
	}
	for _, tt := range tests {
		got, err := config.NormalizeTheme(tt.input)
		if (err == nil) != tt.ok {
			t.Errorf("NormalizeTheme(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("NormalizeTheme(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUIThemeAndLanguagePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := config.UIState{
		WindowWidth:  1200,
		WindowHeight: 800,
		Theme:        "dark",
		Language:     "en-US",
	}
	if err := store.UpdateUI(want); err != nil {
		t.Fatal(err)
	}

	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.UI(); got != want {
		t.Fatalf("UI() = %+v, want %+v", got, want)
	}
}

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "zh-CN", true},
		{"zh-CN", "zh-CN", true},
		{"en-US", "en-US", true},
		{"ja-JP", "", false},
	}
	for _, tt := range tests {
		got, err := config.NormalizeLanguage(tt.input)
		if (err == nil) != tt.ok {
			t.Errorf("NormalizeLanguage(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
