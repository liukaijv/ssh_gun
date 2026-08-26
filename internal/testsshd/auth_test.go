package testsshd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/testsshd"
)

func TestServer_PublicKeyAuth(t *testing.T) {
	root := t.TempDir()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := testsshd.Start(testsshd.Config{
		Root:       root,
		User:       "keyuser",
		PublicKeys: []ssh.PublicKey{signer.PublicKey()},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "keyuser",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = client.Close()
}

func TestServer_WrongPasswordRejected(t *testing.T) {
	root := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{
		Root:     root,
		User:     "test",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	_, err = ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("wrong")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected auth failure")
	}
}
