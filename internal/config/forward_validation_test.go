package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizePortForward_RequiredFields(t *testing.T) {
	t.Parallel()

	validLocal := config.PortForward{
		ID: "f1", ServerID: " srv-1 ", Name: "  mysql  ",
		Type: config.ForwardTypeLocal, LocalPort: 3316,
		RemoteHost: " 127.0.0.1 ", RemotePort: 3306,
	}
	got, err := config.NormalizePortForward(validLocal)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "mysql" || got.ServerID != "srv-1" || got.RemoteHost != "127.0.0.1" {
		t.Fatalf("trimmed = %+v", got)
	}

	cases := []struct {
		name string
		fwd  config.PortForward
		want string
	}{
		{
			"empty name",
			config.PortForward{ID: "1", ServerID: "s", Name: "  ", Type: config.ForwardTypeLocal, LocalPort: 1, RemoteHost: "h", RemotePort: 2},
			"name",
		},
		{
			"empty server",
			config.PortForward{ID: "1", ServerID: "", Name: "n", Type: config.ForwardTypeLocal, LocalPort: 1, RemoteHost: "h", RemotePort: 2},
			"server",
		},
		{
			"local empty remote host",
			config.PortForward{ID: "1", ServerID: "s", Name: "n", Type: config.ForwardTypeLocal, LocalPort: 1, RemoteHost: "  ", RemotePort: 2},
			"remote host",
		},
		{
			"remote empty local addr",
			config.PortForward{ID: "1", ServerID: "s", Name: "n", Type: config.ForwardTypeRemote, LocalAddr: "", LocalPort: 1, RemotePort: 2},
			"local",
		},
		{
			"dynamic empty local port",
			config.PortForward{ID: "1", ServerID: "s", Name: "n", Type: config.ForwardTypeDynamic, LocalPort: 0},
			"local port",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.NormalizePortForward(tt.fwd)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestStore_UpsertPortForward_RejectsInvalidAndDoesNotPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpsertPortForward(config.PortForward{
		ID: "bad", ServerID: "s", Name: "", Type: config.ForwardTypeLocal,
		LocalPort: 1, RemoteHost: "h", RemotePort: 2,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetPortForward("bad"); err == nil {
		t.Fatal("invalid forward should not be persisted")
	}
}
