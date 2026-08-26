package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizeSyncMapping_RequiredFields(t *testing.T) {
	t.Parallel()

	valid := config.SyncMapping{
		ID: "map-1", ServerID: " srv-1 ", Name: "  proj  ",
		LocalPath: ` D:\Work\proj `, RemotePath: " /opt/proj ",
	}
	got, err := config.NormalizeSyncMapping(valid)
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerID != "srv-1" || got.Name != "proj" || got.LocalPath != `D:\Work\proj` || got.RemotePath != "/opt/proj" {
		t.Fatalf("trimmed fields = %+v", got)
	}

	cases := []struct {
		name string
		m    config.SyncMapping
		want string
	}{
		{"empty name", config.SyncMapping{ID: "1", ServerID: "s", Name: "  ", LocalPath: "L", RemotePath: "R"}, "name"},
		{"empty server", config.SyncMapping{ID: "1", ServerID: "", Name: "n", LocalPath: "L", RemotePath: "R"}, "server"},
		{"empty local", config.SyncMapping{ID: "1", ServerID: "s", Name: "n", LocalPath: "\t", RemotePath: "R"}, "local"},
		{"empty remote", config.SyncMapping{ID: "1", ServerID: "s", Name: "n", LocalPath: "L", RemotePath: ""}, "remote"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.NormalizeSyncMapping(tt.m)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestStore_UpsertSyncMapping_RejectsInvalidAndDoesNotPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	err = store.UpsertSyncMapping(config.SyncMapping{
		ID: "bad", ServerID: "s", Name: "", LocalPath: "L", RemotePath: "R",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetSyncMapping("bad"); err == nil {
		t.Fatal("invalid mapping should not be persisted")
	}
}
