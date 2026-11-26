package shellopen

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommandForOS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos     string
		path     string
		wantName string
		wantArgs []string
	}{
		{"windows", `C:\logs`, "explorer.exe", []string{`C:\logs`}},
		{"darwin", "/tmp/logs", "open", []string{"/tmp/logs"}},
		{"linux", "/tmp/logs", "xdg-open", []string{"/tmp/logs"}},
	}
	for _, tt := range tests {
		name, args := commandForOS(tt.goos, tt.path)
		if name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
			t.Errorf("commandForOS(%q, %q) = %q, %#v; want %q, %#v",
				tt.goos, tt.path, name, args, tt.wantName, tt.wantArgs)
		}
	}
}

func TestRevealCommandForOS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos     string
		path     string
		wantName string
		wantArgs []string
	}{
		{"windows", `C:\logs\a.txt`, "explorer.exe", []string{`/select,C:\logs\a.txt`}},
		{"darwin", "/tmp/logs/a.txt", "open", []string{"-R", "/tmp/logs/a.txt"}},
		{"linux", "/tmp/logs/a.txt", "xdg-open", []string{filepath.Dir("/tmp/logs/a.txt")}},
	}
	for _, tt := range tests {
		name, args := revealCommandForOS(tt.goos, tt.path)
		if name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
			t.Errorf("revealCommandForOS(%q, %q) = %q, %#v; want %q, %#v",
				tt.goos, tt.path, name, args, tt.wantName, tt.wantArgs)
		}
	}
}
