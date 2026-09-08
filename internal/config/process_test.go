package config_test

import (
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizeManagedProcess(t *testing.T) {
	got, err := config.NormalizeManagedProcess(config.ManagedProcess{
		ID:      "p1",
		Name:    "  memcached  ",
		Command: "  memcached.exe ",
		Args:    " -p 11211 ",
		WorkDir: " D:\\data ",
		Env: map[string]string{
			" OPENAI_TARGET_API_URL ": " https://tokenhub.tencentmaas.com ",
			"  ":                      "ignored",
			"FOO":                     " bar ",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "memcached" || got.Command != "memcached.exe" || got.Args != "-p 11211" || got.WorkDir != `D:\data` {
		t.Fatalf("got %#v", got)
	}
	if got.Env["OPENAI_TARGET_API_URL"] != "https://tokenhub.tencentmaas.com" || got.Env["FOO"] != "bar" {
		t.Fatalf("env = %#v", got.Env)
	}
	if _, ok := got.Env[""]; ok {
		t.Fatal("empty env key should be dropped")
	}

	cases := []config.ManagedProcess{
		{ID: "", Name: "n", Command: "c"},
		{ID: "1", Name: "  ", Command: "c"},
		{ID: "1", Name: "n", Command: "  "},
	}
	for _, p := range cases {
		if _, err := config.NormalizeManagedProcess(p); err == nil {
			t.Fatalf("expected error for %#v", p)
		}
	}
}

func TestStore_UpsertManagedProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := config.ManagedProcess{
		ID: "proc-1", Name: "headroom", Command: "headroom", Args: "proxy --port 8787",
		Env:     map[string]string{"OPENAI_TARGET_API_URL": "https://tokenhub.tencentmaas.com"},
		Enabled: true, AutoStart: true,
	}
	if err := store.UpsertManagedProcess(p); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetManagedProcess("proc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "headroom" || !got.AutoStart || !got.Enabled {
		t.Fatalf("got %#v", got)
	}
	if got.Env["OPENAI_TARGET_API_URL"] != "https://tokenhub.tencentmaas.com" {
		t.Fatalf("env = %#v", got.Env)
	}

	// Reload from disk to verify TOML round-trip.
	store2, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := store2.GetManagedProcess("proc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Env["OPENAI_TARGET_API_URL"] != "https://tokenhub.tencentmaas.com" {
		t.Fatalf("reloaded env = %#v", got2.Env)
	}

	list := store.ListManagedProcesses()
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if err := store.DeleteManagedProcess("proc-1"); err != nil {
		t.Fatal(err)
	}
	if len(store.ListManagedProcesses()) != 0 {
		t.Fatal("expected empty after delete")
	}
}
