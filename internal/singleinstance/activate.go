package singleinstance

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

const activatePayload = "show\n"

type activateServer struct {
	ln     net.Listener
	mu     sync.Mutex
	closed bool
}

func startActivateServer(onActivate func()) (*activateServer, error) {
	ln, err := listenActivate()
	if err != nil {
		return nil, fmt.Errorf("listen activate: %w", err)
	}
	s := &activateServer{ln: ln}
	go s.serve(onActivate)
	return s, nil
}

func (s *activateServer) serve(onActivate func()) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			continue
		}
		go handleActivateConn(conn, onActivate)
	}
}

func handleActivateConn(conn net.Conn, onActivate func()) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	if line != activatePayload {
		return
	}
	_, _ = conn.Write([]byte("ok\n"))
	if onActivate != nil {
		onActivate()
	}
}

func (s *activateServer) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	_ = s.ln.Close()
}

func notifyActivateImpl() error {
	conn, err := dialActivate()
	if err != nil {
		return fmt.Errorf("notify primary: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte(activatePayload)); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if line != "ok\n" {
		return fmt.Errorf("unexpected activate reply %q", line)
	}
	return nil
}
