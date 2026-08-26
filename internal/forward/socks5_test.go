package forward_test

import (
	"bytes"
	"io"
	"net"
	"testing"

	"ssh_gun/internal/forward"
)

func TestSOCKS5Handshake_NoAuth(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- forward.ServeSOCKS5Handshake(server)
	}()

	// VER NMETHODS METHODS(no-auth)
	if _, err := client.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reply, []byte{0x05, 0x00}) {
		t.Fatalf("handshake reply = %v", reply)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestSOCKS5Handshake_RejectsUnsupportedAuth(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- forward.ServeSOCKS5Handshake(server)
	}()

	if _, err := client.Write([]byte{0x05, 0x01, 0x02}); err != nil { // username/password only
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reply, []byte{0x05, 0xFF}) {
		t.Fatalf("handshake reply = %v", reply)
	}
	if err := <-errCh; err == nil {
		t.Fatal("expected handshake error")
	}
}

func TestSOCKS5Connect_IPv4(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	resultCh := make(chan struct {
		addr string
		err  error
	}, 1)
	go func() {
		addr, err := forward.ServeSOCKS5Connect(server)
		resultCh <- struct {
			addr string
			err  error
		}{addr, err}
	}()

	// CONNECT to 127.0.0.1:8080
	req := []byte{
		0x05, 0x01, 0x00, 0x01,
		127, 0, 0, 1,
		0x1F, 0x90, // 8080
	}
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("connect reply = %v", reply)
	}
	got := <-resultCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.addr != "127.0.0.1:8080" {
		t.Fatalf("addr = %q", got.addr)
	}
}

func TestSOCKS5Connect_Domain(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	resultCh := make(chan struct {
		addr string
		err  error
	}, 1)
	go func() {
		addr, err := forward.ServeSOCKS5Connect(server)
		resultCh <- struct {
			addr string
			err  error
		}{addr, err}
	}()

	host := "example.com"
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, host...)
	req = append(req, 0x01, 0xBB) // 443
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("reply status = %d", reply[1])
	}
	got := <-resultCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.addr != "example.com:443" {
		t.Fatalf("addr = %q", got.addr)
	}
}

func TestSOCKS5Connect_RejectsUDPAssociate(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	resultCh := make(chan error, 1)
	go func() {
		_, err := forward.ServeSOCKS5Connect(server)
		resultCh <- err
	}()

	req := []byte{
		0x05, 0x03, 0x00, 0x01, // UDP ASSOCIATE
		0, 0, 0, 0,
		0, 0,
	}
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x07 { // Command not supported
		t.Fatalf("reply status = %d, want 7", reply[1])
	}
	if err := <-resultCh; err == nil {
		t.Fatal("expected error")
	}
}

func TestSOCKS5Connect_IPv6(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	resultCh := make(chan struct {
		addr string
		err  error
	}, 1)
	go func() {
		addr, err := forward.ServeSOCKS5Connect(server)
		resultCh <- struct {
			addr string
			err  error
		}{addr, err}
	}()

	req := []byte{0x05, 0x01, 0x00, 0x04}
	req = append(req, make([]byte, 15)...)
	req = append(req, 1)          // ::1
	req = append(req, 0x00, 0x50) // 80
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("reply status = %d", reply[1])
	}
	got := <-resultCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.addr != "[::1]:80" {
		t.Fatalf("addr = %q", got.addr)
	}
}
