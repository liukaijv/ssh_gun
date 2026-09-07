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
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "memcached" || got.Command != "memcached.exe" || got.Args != "-p 11211" || got.WorkDir != `D:\data` {
		t.Fatalf("got %#v", got)
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
		ID: "proc-1", Name: "headroom", Command: "headroom", Args: "proxy --port 8787", Enabled: true, AutoStart: true,
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
