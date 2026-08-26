package config_test

import (
	"strings"
	"testing"

	"ssh_gun/internal/config"
)

func sampleFile() config.File {
	return config.File{
		Version: config.CurrentVersion,
		Servers: []config.Server{{
			ID: "srv-1", Name: "dev", Host: "h", Port: 22, User: "u",
			AuthType: "password", Password: "p@ss/word", KeyPassphrase: "phrase",
			HostKeyPolicy: "accept-new",
		}},
		SyncMappings: []config.SyncMapping{{
			ID: "map-1", ServerID: "srv-1", Name: "proj",
			LocalPath: `D:\Work\proj`, RemotePath: "/opt/proj",
		}},
		PortForwards: []config.PortForward{{
			ID: "fwd-1", ServerID: "srv-1", Name: "mysql",
			Type: config.ForwardTypeLocal, LocalPort: 3316, RemotePort: 3306,
		}},
		UI: config.UIState{WindowWidth: 1200, Theme: "dark"},
	}
}

func TestExport_OmittedSecrets(t *testing.T) {
	data, err := config.Export(sampleFile(), "")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if strings.Contains(raw, "p@ss/word") || strings.Contains(raw, "phrase") {
		t.Fatalf("plaintext secret leaked:\n%s", raw)
	}
	if !strings.Contains(raw, `format = "ssh_gun-export"`) {
		t.Fatal("missing format marker")
	}
	if !strings.Contains(raw, `secrets = "omitted"`) {
		t.Fatal("expected secrets omitted")
	}

	got, meta, err := config.ParseExport(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SecretsMode != config.SecretsOmitted {
		t.Fatalf("mode = %q", meta.SecretsMode)
	}
	if got.Servers[0].Password != "" || got.Servers[0].KeyPassphrase != "" {
		t.Fatalf("secrets should be empty: %+v", got.Servers[0])
	}
	if got.Servers[0].Host != "h" || got.SyncMappings[0].LocalPath == "" {
		t.Fatalf("non-secret fields lost: %+v", got)
	}
}

func TestExport_EncryptedRoundTrip(t *testing.T) {
	data, err := config.Export(sampleFile(), "export-pass")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if strings.Contains(raw, "p@ss/word") {
		t.Fatal("password in plaintext export")
	}
	if !strings.Contains(raw, `secrets = "encrypted"`) {
		t.Fatal("expected encrypted mode")
	}

	got, meta, err := config.ParseExport(data, "export-pass")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SecretsMode != config.SecretsEncrypted {
		t.Fatalf("mode = %q", meta.SecretsMode)
	}
	if got.Servers[0].Password != "p@ss/word" || got.Servers[0].KeyPassphrase != "phrase" {
		t.Fatalf("secrets = %+v", got.Servers[0])
	}
}

func TestParseExport_WrongPassphrase(t *testing.T) {
	data, err := config.Export(sampleFile(), "right")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = config.ParseExport(data, "wrong")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseExport_EncryptedRequiresPassphrase(t *testing.T) {
	data, err := config.Export(sampleFile(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = config.ParseExport(data, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseExport_RejectsRuntimeConfig(t *testing.T) {
	runtimeTOML := `
version = 1
[[server]]
id = "srv-1"
name = "dev"
host = "h"
port = 22
user = "u"
auth_type = "password"
password = "dpapi-looking-blob"
`
	_, _, err := config.ParseExport([]byte(runtimeTOML), "")
	if err == nil {
		t.Fatal("expected reject")
	}
}
