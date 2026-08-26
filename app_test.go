package main

import (
	"os"
	"path/filepath"
	"testing"
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
