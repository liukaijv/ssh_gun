package config_test

import (
	"testing"

	"ssh_gun/internal/config"
)

func TestNormalizePortForward(t *testing.T) {
	t.Parallel()

	t.Run("empty type becomes local", func(t *testing.T) {
		got, err := config.NormalizePortForward(config.PortForward{
			ID: "f1", ServerID: "s", Name: "n", LocalPort: 1000, RemotePort: 2000, RemoteHost: "127.0.0.1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Type != config.ForwardTypeLocal {
			t.Fatalf("Type = %q, want local", got.Type)
		}
		if got.LocalAddr != "127.0.0.1" || got.RemoteHost != "127.0.0.1" {
			t.Fatalf("defaults: %+v", got)
		}
	})

	t.Run("dynamic skips remote port", func(t *testing.T) {
		got, err := config.NormalizePortForward(config.PortForward{
			ID: "f2", ServerID: "s", Name: "socks", Type: config.ForwardTypeDynamic, LocalPort: 1080,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.RemotePort != 0 {
			t.Fatalf("RemotePort = %d, want 0", got.RemotePort)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		_, err := config.NormalizePortForward(config.PortForward{
			ID: "f3", ServerID: "s", Name: "n", Type: "udp", LocalPort: 1, RemotePort: 1, RemoteHost: "h",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("local requires remote port", func(t *testing.T) {
		_, err := config.NormalizePortForward(config.PortForward{
			ID: "f4", ServerID: "s", Name: "n", Type: config.ForwardTypeLocal, LocalPort: 1, RemoteHost: "h",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
