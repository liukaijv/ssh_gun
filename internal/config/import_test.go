package config_test

import (
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestStore_ExportImportMerge(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{
		ID: "srv-1", Name: "old", Host: "h1", Port: 22, User: "u",
		AuthType: "password", Password: "keep-me", HostKeyPolicy: "accept-new",
	})
	_ = store.UpdateUI(config.UIState{WindowWidth: 900, Theme: "dark"})

	incoming := config.File{
		Servers: []config.Server{
			{ID: "srv-1", Name: "new", Host: "h2", Port: 22, User: "u", AuthType: "password", Password: "imported", HostKeyPolicy: "accept-new"},
			{ID: "srv-2", Name: "extra", Host: "h3", Port: 22, User: "u", AuthType: "password", Password: "x", HostKeyPolicy: "accept-new"},
		},
		UI: config.UIState{WindowWidth: 1, Theme: "light"},
	}
	sum, err := store.ImportMerge(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ServersUpdated != 1 || sum.ServersAdded != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	srv, _ := store.GetServer("srv-1")
	if srv.Name != "new" || srv.Password != "imported" {
		t.Fatalf("merged server = %+v", srv)
	}
	if store.UI().WindowWidth != 900 || store.UI().Theme != "dark" {
		t.Fatalf("UI should be preserved: %+v", store.UI())
	}
}

func TestStore_ImportReplace(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{ID: "old", Name: "old", Host: "h", Port: 22, User: "u", AuthType: "password", Password: "a"})
	_ = store.UpsertPortForward(config.PortForward{ID: "fwd-old", ServerID: "old", Name: "x", LocalPort: 1, RemoteHost: "127.0.0.1", RemotePort: 2})

	incoming := config.File{
		Servers: []config.Server{{ID: "only", Name: "only", Host: "n", Port: 22, User: "u", AuthType: "password", Password: "b"}},
		UI:      config.UIState{WindowWidth: 800},
	}
	sum, err := store.ImportReplace(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ServersAdded != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if _, err := store.GetServer("only"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetServer("old"); err == nil {
		t.Fatal("old server should be gone")
	}
	if len(store.ListPortForwards()) != 0 {
		t.Fatal("forwards should be cleared")
	}
	if store.UI().WindowWidth != 800 {
		t.Fatalf("UI = %+v", store.UI())
	}
}

func TestStore_ExportBytesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{
		ID: "srv-1", Name: "dev", Host: "h", Port: 22, User: "u",
		AuthType: "password", Password: "secret", HostKeyPolicy: "accept-new",
	})
	_ = store.UpdateSyncBackend("rsync")
	data, err := store.ExportBytes("pw")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := config.ParseExport(data, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if got.Servers[0].Password != "secret" {
		t.Fatalf("password = %q", got.Servers[0].Password)
	}
	if got.SyncBackend != "rsync" {
		t.Fatalf("sync backend = %q", got.SyncBackend)
	}
}
