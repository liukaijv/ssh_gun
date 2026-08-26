package syncengine

import "testing"

func TestPOSIXShellQuote(t *testing.T) {
	tests := map[string]string{
		"":              "''",
		"plain":         "'plain'",
		"hello world":   "'hello world'",
		"it's ready":    "'it'\"'\"'s ready'",
		"$HOME; rm -rf": "'$HOME; rm -rf'",
	}
	for input, want := range tests {
		if got := POSIXShellQuote(input); got != want {
			t.Errorf("POSIXShellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}
