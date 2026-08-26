package config_test

import (
	"path/filepath"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizeSyncBackend(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "sftp", true},
		{"sftp", "sftp", true},
		{"rsync", "rsync", true},
		{"SFTP", "sftp", true},
		{"ftp", "", false},
	}
	for _, tt := range tests {
		got, err := config.NormalizeSyncBackend(tt.input)
		if (err == nil) != tt.ok {
			t.Errorf("NormalizeSyncBackend(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("NormalizeSyncBackend(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStoreSyncBackendPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.SyncBackend(); got != "sftp" {
		t.Fatalf("default SyncBackend() = %q, want sftp", got)
	}
	if err := store.UpdateSyncBackend("rsync"); err != nil {
		t.Fatal(err)
	}

	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.SyncBackend(); got != "rsync" {
		t.Fatalf("reopened SyncBackend() = %q, want rsync", got)
	}
	if err := reopened.UpdateSyncBackend("ftp"); err == nil {
		t.Fatal("invalid backend should fail")
	}
}

func TestImportSyncBackendSemantics(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSyncBackend("rsync"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ImportMerge(config.File{SyncBackend: "sftp"}); err != nil {
		t.Fatal(err)
	}
	if got := store.SyncBackend(); got != "rsync" {
		t.Fatalf("merge changed backend to %q", got)
	}

	if _, err := store.ImportReplace(config.File{SyncBackend: "sftp"}); err != nil {
		t.Fatal(err)
	}
	if got := store.SyncBackend(); got != "sftp" {
		t.Fatalf("replace backend = %q, want sftp", got)
	}
}
