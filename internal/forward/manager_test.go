package forward_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"ssh_gun/internal/config"
	"ssh_gun/internal/forward"
	"ssh_gun/internal/sshclient"
	"ssh_gun/internal/testsshd"
)

func TestManager_BidirectionalData(t *testing.T) {
	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	localPort := unusedPort(t)

	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)
	fwd := newForward(t, "bidirectional", localPort, echoAddr)
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(fwd.ID) })

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local forward: %v", err)
	}
	defer conn.Close()

	want := []byte("bidirectional data")
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

func TestManager_ConcurrentConnections(t *testing.T) {
	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)
	fwd := newForward(t, "concurrent", unusedPort(t), echoAddr)
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(fwd.ID) })

	const connections = 8
	var wg sync.WaitGroup
	errs := make(chan error, connections)
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)))
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()
			message := []byte("connection-" + strconv.Itoa(i))
			if _, err := conn.Write(message); err != nil {
				errs <- err
				return
			}
			got := make([]byte, len(message))
			if _, err := io.ReadFull(conn, got); err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(got, message) {
				errs <- io.ErrUnexpectedEOF
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent connection: %v", err)
	}
}

func TestManager_StopRejectsFurtherConnections(t *testing.T) {
	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)
	fwd := newForward(t, "stop", unusedPort(t), echoAddr)
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := manager.Stop(fwd.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)), 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial succeeded after stop")
	}
}

func TestManager_PortAlreadyInUse(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })

	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)
	fwd := newForward(t, "occupied", occupied.Addr().(*net.TCPAddr).Port, echoAddr)
	if err := manager.Start(context.Background(), fwd, srv); err == nil {
		t.Fatal("start succeeded with occupied local port")
	}
}

func TestManager_Status(t *testing.T) {
	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)
	fwd := newForward(t, "status", unusedPort(t), echoAddr)
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(fwd.ID) })

	if got := manager.Status(fwd.ID); !got.Running || got.ActiveConns != 0 || got.Err != "" {
		t.Fatalf("initial status = %+v", got)
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return manager.Status(fwd.ID).ActiveConns == 1 })
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return manager.Status(fwd.ID).ActiveConns == 0 })

	if err := manager.Stop(fwd.ID); err != nil {
		t.Fatal(err)
	}
	if got := manager.Status(fwd.ID); got.Running || got.ActiveConns != 0 || got.Err != "" {
		t.Fatalf("stopped status = %+v", got)
	}
}

func TestManager_DynamicSOCKS5(t *testing.T) {
	echoAddr := startEchoServer(t)
	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)

	localPort := unusedPort(t)
	fwd := config.PortForward{
		ID:        "dynamic",
		ServerID:  t.Name(),
		Name:      "dynamic",
		Type:      config.ForwardTypeDynamic,
		LocalAddr: "127.0.0.1",
		LocalPort: localPort,
	}
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(fwd.ID) })

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)), 5*time.Second)
	if err != nil {
		t.Fatalf("dial socks: %v", err)
	}
	defer conn.Close()

	if err := socks5DialThrough(conn, echoAddr); err != nil {
		t.Fatalf("socks handshake: %v", err)
	}

	want := []byte("socks-dynamic")
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

func TestManager_RemoteForward(t *testing.T) {
	echoAddr := startEchoServer(t)
	echoHost, echoPortText, err := net.SplitHostPort(echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	echoPort, err := strconv.Atoi(echoPortText)
	if err != nil {
		t.Fatal(err)
	}

	srv := startSSHServer(t)
	pool := sshclient.NewPool()
	t.Cleanup(pool.Close)
	manager := forward.NewManager(pool)

	remotePort := unusedPort(t)
	fwd := config.PortForward{
		ID:         "remote",
		ServerID:   t.Name(),
		Name:       "remote",
		Type:       config.ForwardTypeRemote,
		LocalAddr:  echoHost,
		LocalPort:  echoPort,
		RemoteHost: "127.0.0.1",
		RemotePort: remotePort,
	}
	if err := manager.Start(context.Background(), fwd, srv); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(fwd.ID) })

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 5*time.Second)
	if err != nil {
		t.Fatalf("dial remote listen: %v", err)
	}
	defer conn.Close()

	want := []byte("remote-forward")
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

func socks5DialThrough(conn net.Conn, dest string) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	hs := make([]byte, 2)
	if _, err := io.ReadFull(conn, hs); err != nil {
		return err
	}
	if hs[0] != 0x05 || hs[1] != 0x00 {
		return fmt.Errorf("handshake reply %v", hs)
	}

	host, portText, err := net.SplitHostPort(dest)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return fmt.Errorf("need ipv4 dest for test, got %s", host)
	}
	req := []byte{0x05, 0x01, 0x00, 0x01, ip[0], ip[1], ip[2], ip[3], byte(port >> 8), byte(port)}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("connect reply status %d", reply[1])
	}
	return nil
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}

func startEchoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().String()
}

func startSSHServer(t *testing.T) config.Server {
	t.Helper()
	sshd, err := testsshd.Start(testsshd.Config{
		Root:     t.TempDir(),
		User:     "test",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sshd.Close() })
	host, portText, err := net.SplitHostPort(sshd.Addr())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return config.Server{
		ID:            t.Name(),
		Host:          host,
		Port:          port,
		User:          "test",
		AuthType:      "password",
		Password:      "secret",
		HostKeyPolicy: "accept-new",
	}
}

func unusedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func newForward(t *testing.T, id string, localPort int, remoteAddr string) config.PortForward {
	t.Helper()
	remoteHost, remotePortText, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		t.Fatal(err)
	}
	remotePort, err := strconv.Atoi(remotePortText)
	if err != nil {
		t.Fatal(err)
	}
	return config.PortForward{
		ID:         id,
		ServerID:   t.Name(),
		Name:       id,
		LocalAddr:  "127.0.0.1",
		LocalPort:  localPort,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
	}
}
