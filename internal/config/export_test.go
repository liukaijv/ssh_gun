package config_test

import (
	"strings"
	"testing"
	"time"

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
			AutoSync: true, LastSyncAt: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			LastSyncResult: "ok", LastSyncError: "should-not-export",
		}},
		PortForwards: []config.PortForward{{
			ID: "fwd-1", ServerID: "srv-1", Name: "mysql",
			Type: config.ForwardTypeLocal, LocalPort: 3316, RemotePort: 3306,
			AutoStart: true,
		}},
		Processes: []config.ManagedProcess{{
			ID: "proc-1", Name: "demo", Command: "demo.exe", Enabled: true, AutoStart: true,
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

func TestExport_DisablesAutostartAndClearsLastSync(t *testing.T) {
	src := sampleFile()
	data, err := config.Export(src, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := config.ParseExport(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncMappings[0].AutoSync {
		t.Fatal("exported mapping AutoSync should be false")
	}
	m := got.SyncMappings[0]
	if !m.LastSyncAt.IsZero() || m.LastSyncResult != "" || m.LastSyncError != "" {
		t.Fatalf("LastSync* should be cleared: %+v", m)
	}
	if got.PortForwards[0].AutoStart {
		t.Fatal("exported forward AutoStart should be false")
	}
	if got.Processes[0].AutoStart {
		t.Fatal("exported process AutoStart should be false")
	}
	// Source file used for export must not be mutated.
	if !src.SyncMappings[0].AutoSync || src.SyncMappings[0].LastSyncError == "" {
		t.Fatalf("source mapping mutated: %+v", src.SyncMappings[0])
	}
	if !src.PortForwards[0].AutoStart || !src.Processes[0].AutoStart {
		t.Fatal("source autostart flags mutated")
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
