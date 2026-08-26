package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizeServer_RequiredFields(t *testing.T) {
	t.Parallel()

	valid := config.Server{
		ID: "srv-1", Name: "  dev  ", Host: " 127.0.0.1 ", Port: 22, User: " abc ",
		AuthType: "password",
	}
	got, err := config.NormalizeServer(valid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "dev" || got.Host != "127.0.0.1" || got.User != "abc" {
		t.Fatalf("trimmed fields = %+v", got)
	}

	cases := []struct {
		name string
		srv  config.Server
		want string
	}{
		{"empty name", config.Server{ID: "1", Name: "  ", Host: "h", Port: 22, User: "u"}, "name"},
		{"empty host", config.Server{ID: "1", Name: "n", Host: "", Port: 22, User: "u"}, "host"},
		{"empty user", config.Server{ID: "1", Name: "n", Host: "h", Port: 22, User: " \t"}, "user"},
		{"port zero", config.Server{ID: "1", Name: "n", Host: "h", Port: 0, User: "u"}, "port"},
		{"port high", config.Server{ID: "1", Name: "n", Host: "h", Port: 65536, User: "u"}, "port"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.NormalizeServer(tt.srv)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestStore_UpsertServer_RejectsInvalidAndDoesNotPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	err = store.UpsertServer(config.Server{
		ID: "bad", Name: "", Host: "h", Port: 22, User: "u", AuthType: "password",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetServer("bad"); err == nil {
		t.Fatal("invalid server should not be persisted")
	}
}

func TestStore_UpsertServer_NoLongerDefaultsPortZero(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpsertServer(config.Server{
		ID: "srv", Name: "n", Host: "h", Port: 0, User: "u", AuthType: "password",
	})
	if err == nil {
		t.Fatal("port 0 should be rejected, not defaulted to 22")
	}
}
