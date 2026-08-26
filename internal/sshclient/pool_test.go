package sshclient_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/config"
	"ssh_gun/internal/sshclient"
	"ssh_gun/internal/testsshd"
)

func TestPool_DialPasswordAndReuse(t *testing.T) {
	root := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{Root: root, User: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	host, port := mustHostPort(t, srv.Addr())
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)

	cfg := config.Server{
		ID: "s1", Host: host, Port: port, User: "u",
		AuthType: "password", Password: "p", HostKeyPolicy: "accept-new",
	}
	c1, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	c2, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected connection reuse")
	}
}

func TestPool_WrongPassword(t *testing.T) {
	root := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{Root: root, User: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	host, port := mustHostPort(t, srv.Addr())
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)

	_, err = pool.Get(context.Background(), config.Server{
		ID: "s1", Host: host, Port: port, User: "u",
		AuthType: "password", Password: "wrong", HostKeyPolicy: "accept-new",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestPool_PublicKey(t *testing.T) {
	root := t.TempDir()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	b, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(b), 0o600); err != nil {
		t.Fatal(err)
	}

	srv, err := testsshd.Start(testsshd.Config{
		Root: root, User: "keyuser", PublicKeys: []ssh.PublicKey{signer.PublicKey()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	host, port := mustHostPort(t, srv.Addr())

	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	_, err = pool.Get(context.Background(), config.Server{
		ID: "s1", Host: host, Port: port, User: "keyuser",
		AuthType: "key", KeyPath: keyPath, HostKeyPolicy: "accept-new",
	})
	if err != nil {
		t.Fatalf("key dial: %v", err)
	}
}

func TestClient_TestConnection(t *testing.T) {
	root := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{Root: root, User: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	host, port := mustHostPort(t, srv.Addr())

	ok, msg, err := sshclient.TestConnection(context.Background(), config.Server{
		ID: "s1", Host: host, Port: port, User: "u",
		AuthType: "password", Password: "p", HostKeyPolicy: "accept-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("not ok: %s", msg)
	}
}

func TestNewSFTP_FromSSHClient(t *testing.T) {
	root := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{Root: root, User: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	host, port := mustHostPort(t, srv.Addr())

	client, err := sshclient.Dial(context.Background(), config.Server{
		ID: "s1", Host: host, Port: port, User: "u",
		AuthType: "password", Password: "p", HostKeyPolicy: "accept-new",
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sftpClient, err := sshclient.NewSFTP(client)
	if err != nil {
		t.Fatalf("new sftp: %v", err)
	}
	t.Cleanup(func() { _ = sftpClient.Close() })
	if _, err := sftpClient.Stat("."); err != nil {
		t.Fatalf("stat through sftp: %v", err)
	}
}

func mustHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
