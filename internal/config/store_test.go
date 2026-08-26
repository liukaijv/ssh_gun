package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"ssh_gun/internal/config"
)

func TestStore_RoundTrip_WindowsPathAndSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	store, err := config.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	srv := config.Server{
		ID:            "srv-1",
		Name:          "dev",
		Host:          "192.168.0.11",
		Port:          22,
		User:          "root",
		AuthType:      "password",
		Password:      "p@ss/word",
		HostKeyPolicy: "accept-new",
	}
	if err := store.UpsertServer(srv); err != nil {
		t.Fatalf("upsert server: %v", err)
	}

	mapCfg := config.SyncMapping{
		ID:         "map-1",
		ServerID:   "srv-1",
		Name:       "proj",
		LocalPath:  `D:\Work\proj`,
		RemotePath: "/opt/proj",
		Excludes:   []string{".git", "node_modules", "*.log"},
		AutoSync:   true,
		DebounceMs: 500,
		Backend:    "sftp",
		Enabled:    true,
	}
	if err := store.UpsertSyncMapping(mapCfg); err != nil {
		t.Fatalf("upsert mapping: %v", err)
	}

	fwd := config.PortForward{
		ID:         "fwd-1",
		ServerID:   "srv-1",
		Name:       "mysql",
		Type:       config.ForwardTypeLocal,
		LocalAddr:  "127.0.0.1",
		LocalPort:  3316,
		RemoteHost: "127.0.0.1",
		RemotePort: 3306,
		AutoStart:  true,
	}
	if err := store.UpsertPortForward(fwd); err != nil {
		t.Fatalf("upsert forward: %v", err)
	}

	// Reload from disk
	store2, err := config.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	gotSrv, err := store2.GetServer("srv-1")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if gotSrv.Password != "p@ss/word" {
		t.Fatalf("password = %q, want plaintext after decrypt", gotSrv.Password)
	}

	// File on disk must not contain plaintext password
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "p@ss/word") {
		t.Fatalf("config file contains plaintext password:\n%s", raw)
	}
	if !strings.Contains(string(raw), `local_path`) {
		t.Fatalf("expected local_path in toml:\n%s", raw)
	}

	gotMap, err := store2.GetSyncMapping("map-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotMap.LocalPath != `D:\Work\proj` {
		t.Fatalf("local_path = %q, want D:\\Work\\proj", gotMap.LocalPath)
	}

	gotFwd, err := store2.GetPortForward("fwd-1")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(fwd, gotFwd, cmpopts.IgnoreFields(config.PortForward{})); diff != "" {
		t.Fatalf("forward mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_DeleteServer_BlockedWhenReferenced(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{ID: "srv-1", Name: "dev", Host: "h", Port: 22, User: "u", AuthType: "password"})
	_ = store.UpsertSyncMapping(config.SyncMapping{ID: "map-1", ServerID: "srv-1", Name: "proj", LocalPath: "C:\\a", RemotePath: "/a"})
	_ = store.UpsertPortForward(config.PortForward{ID: "fwd-1", ServerID: "srv-1", Name: "mysql", LocalPort: 3316, RemoteHost: "127.0.0.1", RemotePort: 3306})

	err = store.DeleteServer("srv-1")
	if err == nil {
		t.Fatal("expected error when server is referenced")
	}
	refErr, ok := err.(*config.InUseError)
	if !ok {
		t.Fatalf("want *InUseError, got %T: %v", err, err)
	}
	if len(refErr.Mappings) != 1 || refErr.Mappings[0] != "proj" {
		t.Fatalf("mappings = %#v", refErr.Mappings)
	}
	if len(refErr.Forwards) != 1 || refErr.Forwards[0] != "mysql" {
		t.Fatalf("forwards = %#v", refErr.Forwards)
	}
}

func TestStore_MaskForUI_HidesSecrets(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{
		ID: "srv-1", Name: "dev", Host: "h", Port: 22, User: "u",
		AuthType: "password", Password: "secret", KeyPassphrase: "phrase",
	})
	ui := store.ListServersForUI()
	if len(ui) != 1 {
		t.Fatalf("len=%d", len(ui))
	}
	if ui[0].Password != "" && ui[0].Password != config.SecretMask {
		t.Fatalf("password leaked: %q", ui[0].Password)
	}
	if ui[0].HasPassword != true {
		t.Fatal("HasPassword should be true")
	}
	if ui[0].KeyPassphrase != "" && ui[0].KeyPassphrase != config.SecretMask {
		t.Fatalf("passphrase leaked: %q", ui[0].KeyPassphrase)
	}
}

func TestStore_CorruptFile_FallsBackToBak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertServer(config.Server{ID: "srv-1", Name: "dev", Host: "h", Port: 22, User: "u", AuthType: "password", Password: "x"})
	// Second save creates .bak from the first good file
	_ = store.UpsertServer(config.Server{ID: "srv-1", Name: "dev2", Host: "h", Port: 22, User: "u", AuthType: "password", Password: "x"})

	// Corrupt main file; bak should still exist from last save
	if err := os.WriteFile(path, []byte("not toml {{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	store2, err := config.Open(path)
	if err != nil {
		t.Fatalf("open after corrupt: %v", err)
	}
	got, err := store2.GetServer("srv-1")
	if err != nil {
		t.Fatalf("expected recovery from bak: %v", err)
	}
	if got.Name != "dev" {
		t.Fatalf("recovered name = %q, want previous bak value 'dev'", got.Name)
	}
}
