package rshbridge

import (
	"strings"
	"testing"
)

func TestIsBridgeModeFindsMarker(t *testing.T) {
	if !IsBridgeMode([]string{"ssh_gun.exe", "--rsh-bridge", "ignored", "rsync"}) {
		t.Fatal("bridge marker was not detected")
	}
	if IsBridgeMode([]string{"ssh_gun.exe"}) {
		t.Fatal("ordinary GUI invocation detected as bridge mode")
	}
}

func TestParseReadsEnvironmentAndRemovesRsyncHost(t *testing.T) {
	env := map[string]string{
		"SSH_GUN_HOST":            "real.example",
		"SSH_GUN_PORT":            "2222",
		"SSH_GUN_USER":            "alice",
		"SSH_GUN_AUTH_TYPE":       "key",
		"SSH_GUN_KEY_PATH":        `C:\keys\id_ed25519`,
		"SSH_GUN_KEY_PASSPHRASE":  "secret",
		"SSH_GUN_HOST_KEY_POLICY": "accept-new",
	}
	cfg, command, err := Parse(
		[]string{"ssh_gun.exe", "--rsh-bridge", "-l", "ignored-user", "user@ignored", "rsync", "--server", "-logDtpre.iLsfxCIvu", "."},
		func(key string) string { return env[key] },
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "real.example" || cfg.Port != 2222 || cfg.User != "alice" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.AuthType != "key" || cfg.KeyPath != `C:\keys\id_ed25519` || cfg.KeyPassphrase != "secret" {
		t.Fatalf("key config = %+v", cfg)
	}
	wantCommand := "rsync --server -logDtpre.iLsfxCIvu ."
	if command != wantCommand {
		t.Fatalf("command = %q, want %q", command, wantCommand)
	}
}

func TestParseDefaultsPortAndValidatesRequiredValues(t *testing.T) {
	env := map[string]string{
		"SSH_GUN_HOST":     "example.test",
		"SSH_GUN_USER":     "alice",
		"SSH_GUN_PASSWORD": "password",
	}
	cfg, _, err := Parse(
		[]string{"ssh_gun.exe", "--rsh-bridge", "ignored", "rsync --server ."},
		func(key string) string { return env[key] },
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 22 || cfg.AuthType != "password" {
		t.Fatalf("defaults = %+v", cfg)
	}

	delete(env, "SSH_GUN_HOST")
	_, _, err = Parse([]string{"ssh_gun.exe", "--rsh-bridge", "ignored", "true"}, func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "SSH_GUN_HOST") {
		t.Fatalf("missing host error = %v", err)
	}
}
