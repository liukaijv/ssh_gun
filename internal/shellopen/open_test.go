package shellopen

import (
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
