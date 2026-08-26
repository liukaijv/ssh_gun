package testsshd_test

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/testsshd"
)

func TestServer_PasswordAuth_ListAndReadWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := testsshd.Start(testsshd.Config{
		Root:     root,
		User:     "test",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		t.Fatalf("sftp: %v", err)
	}
	t.Cleanup(func() { _ = sftpClient.Close() })

	entries, err := sftpClient.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if diff := cmp.Diff([]string{"hello.txt"}, names); diff != "" {
		t.Fatalf("readdir mismatch (-want +got):\n%s", diff)
	}

	got, err := sftpClient.Open("hello.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, err := io.ReadAll(got)
	_ = got.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(data, []byte("hello")) {
		t.Fatalf("content = %q, want hello", data)
	}

	f, err := sftpClient.Create("out.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.Write([]byte("written")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	local, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("read local: %v", err)
	}
	if !bytes.Equal(local, []byte("written")) {
		t.Fatalf("local content = %q, want written", local)
	}
}

func TestServer_DirectTCPIP(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echo.Close() })
	go func() {
		conn, err := echo.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	srv, err := testsshd.Start(testsshd.Config{
		Root:     t.TempDir(),
		User:     "test",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial ssh: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	conn, err := client.Dial("tcp", echo.Addr().String())
	if err != nil {
		t.Fatalf("dial direct-tcpip: %v", err)
	}
	defer conn.Close()

	want := []byte("through ssh")
	if _, err := conn.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
}

func TestServer_RemoteTCPIPForward(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echoLn.Close() })
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()

	srv, err := testsshd.Start(testsshd.Config{
		Root:     t.TempDir(),
		User:     "test",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	remoteLn, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("remote listen: %v", err)
	}
	t.Cleanup(func() { _ = remoteLn.Close() })

	go func() {
		for {
			incoming, err := remoteLn.Accept()
			if err != nil {
				return
			}
			go func() {
				defer incoming.Close()
				local, err := net.Dial("tcp", echoLn.Addr().String())
				if err != nil {
					return
				}
				defer local.Close()
				done := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(local, incoming); done <- struct{}{} }()
				go func() { _, _ = io.Copy(incoming, local); done <- struct{}{} }()
				<-done
			}()
		}
	}()

	conn, err := net.DialTimeout("tcp", remoteLn.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial remote: %v", err)
	}
	defer conn.Close()

	want := []byte("remote-tcpip")
	if _, err := conn.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
