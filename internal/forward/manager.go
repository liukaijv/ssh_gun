// Package forward manages local, remote, and dynamic (SOCKS5) TCP forwards over SSH.
package forward

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"

	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/config"
	"ssh_gun/internal/sshclient"
)

// Status describes the current state of a port forward.
type Status struct {
	Running     bool
	ActiveConns int
	Err         string
}

// Option configures a Manager.
type Option func(*Manager)

// WithLogger sets the logger used for Running state transitions.
func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		if logger != nil {
			m.log = logger
		}
	}
}

// Manager starts and stops TCP / SOCKS port forwards over SSH.
type Manager struct {
	pool *sshclient.Pool
	log  *slog.Logger

	mu       sync.Mutex
	forwards map[string]*runningForward
}

type runningForward struct {
	id       string
	name     string
	fwdType  string
	listener net.Listener
	running  bool
	active   int
	err      string

	wg    sync.WaitGroup
	conns map[*connectionPair]struct{}
}

type connectionPair struct {
	mu     sync.Mutex
	local  net.Conn
	remote net.Conn
	closed bool
}

// NewManager creates a port-forward manager backed by pool.
func NewManager(pool *sshclient.Pool, options ...Option) *Manager {
	manager := &Manager{
		pool:     pool,
		log:      slog.New(slog.DiscardHandler),
		forwards: make(map[string]*runningForward),
	}
	for _, option := range options {
		if option != nil {
			option(manager)
		}
	}
	return manager
}

// Start begins a local, remote, or dynamic (SOCKS5) forward.
func (m *Manager) Start(ctx context.Context, fwd config.PortForward, srv config.Server) error {
	normalized, err := config.NormalizePortForward(fwd)
	if err != nil {
		return err
	}
	fwd = normalized

	m.mu.Lock()
	defer m.mu.Unlock()

	if current, ok := m.forwards[fwd.ID]; ok && current.running {
		return fmt.Errorf("port forward %q is already running", fwd.ID)
	}

	client, err := m.pool.Get(ctx, srv)
	if err != nil {
		return fmt.Errorf("connect ssh server: %w", err)
	}

	var listener net.Listener
	switch fwd.Type {
	case config.ForwardTypeRemote:
		remoteAddr := net.JoinHostPort(fwd.RemoteHost, strconv.Itoa(fwd.RemotePort))
		listener, err = client.Listen("tcp", remoteAddr)
		if err != nil {
			return fmt.Errorf("remote listen %s: %w", remoteAddr, err)
		}
	default: // local + dynamic
		localAddr := net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort))
		listener, err = net.Listen("tcp", localAddr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", localAddr, err)
		}
	}

	running := &runningForward{
		id:       fwd.ID,
		name:     fwd.Name,
		fwdType:  fwd.Type,
		listener: listener,
		running:  true,
		conns:    make(map[*connectionPair]struct{}),
	}
	m.forwards[fwd.ID] = running
	running.wg.Add(1)

	switch fwd.Type {
	case config.ForwardTypeRemote:
		go m.acceptLoop(running, func(pair *connectionPair) {
			m.pipeDialNet(fwd.ID, running, pair, net.JoinHostPort(fwd.LocalAddr, strconv.Itoa(fwd.LocalPort)))
		})
	case config.ForwardTypeDynamic:
		go m.acceptLoop(running, func(pair *connectionPair) {
			m.handleDynamic(fwd.ID, running, pair, client)
		})
	default:
		remoteAddr := net.JoinHostPort(fwd.RemoteHost, strconv.Itoa(fwd.RemotePort))
		go m.acceptLoop(running, func(pair *connectionPair) {
			m.pipeDialSSH(fwd.ID, running, pair, client, remoteAddr)
		})
	}
	m.log.Info("port forward started", "id", fwd.ID, "name", fwd.Name, "type", fwd.Type)
	return nil
}

// Stop closes the listener and all active connections for id.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	running, ok := m.forwards[id]
	if !ok || !running.running {
		m.mu.Unlock()
		return nil
	}
	running.running = false
	name := running.name
	fwdType := running.fwdType
	pairs := make([]*connectionPair, 0, len(running.conns))
	for pair := range running.conns {
		pairs = append(pairs, pair)
	}
	m.mu.Unlock()

	err := running.listener.Close()
	for _, pair := range pairs {
		pair.close()
	}
	running.wg.Wait()
	m.log.Info("port forward stopped", "id", id, "name", name, "type", fwdType)
	return err
}

// Status reports the state of id. Unknown IDs have the zero status.
func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	running, ok := m.forwards[id]
	if !ok {
		return Status{}
	}
	return Status{
		Running:     running.running,
		ActiveConns: running.active,
		Err:         running.err,
	}
}

func (m *Manager) acceptLoop(running *runningForward, handle func(*connectionPair)) {
	defer running.wg.Done()
	for {
		conn, err := running.listener.Accept()
		if err != nil {
			m.mu.Lock()
			wasRunning := running.running
			if wasRunning {
				running.running = false
				running.err = err.Error()
			}
			id := running.id
			name := running.name
			fwdType := running.fwdType
			m.mu.Unlock()
			if wasRunning {
				m.log.Info("port forward stopped", "id", id, "name", name, "type", fwdType, "err", err)
			}
			return
		}

		pair := &connectionPair{local: conn}
		m.mu.Lock()
		if !running.running {
			m.mu.Unlock()
			pair.close()
			return
		}
		running.conns[pair] = struct{}{}
		running.active++
		clientAddr := conn.RemoteAddr().String()
		m.log.Info("port forward connection opened",
			"id", running.id,
			"name", running.name,
			"type", running.fwdType,
			"active", running.active,
			"client", clientAddr,
		)
		m.mu.Unlock()

		running.wg.Add(1)
		go func() {
			defer running.wg.Done()
			defer func() {
				pair.close()
				m.mu.Lock()
				delete(running.conns, pair)
				running.active--
				m.log.Info("port forward connection closed",
					"id", running.id,
					"name", running.name,
					"type", running.fwdType,
					"active", running.active,
					"client", clientAddr,
				)
				m.mu.Unlock()
			}()
			handle(pair)
		}()
	}
}

func (m *Manager) handleDynamic(id string, running *runningForward, pair *connectionPair, client *ssh.Client) {
	if err := ServeSOCKS5Handshake(pair.local); err != nil {
		m.setErr(id, running, err)
		return
	}
	dest, err := ServeSOCKS5Connect(pair.local)
	if err != nil {
		m.setErr(id, running, err)
		return
	}
	m.pipeDialSSH(id, running, pair, client, dest)
}

func (m *Manager) pipeDialSSH(id string, running *runningForward, pair *connectionPair, client *ssh.Client, remoteAddr string) {
	remote, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		m.setErr(id, running, err)
		return
	}
	if !pair.setRemote(remote) {
		return
	}
	bidirectionalCopy(pair)
}

func (m *Manager) pipeDialNet(id string, running *runningForward, pair *connectionPair, addr string) {
	remote, err := net.Dial("tcp", addr)
	if err != nil {
		m.setErr(id, running, err)
		return
	}
	if !pair.setRemote(remote) {
		return
	}
	bidirectionalCopy(pair)
}

func bidirectionalCopy(pair *connectionPair) {
	done := make(chan struct{}, 2)
	go copyConnection(done, pair.remote, pair.local)
	go copyConnection(done, pair.local, pair.remote)
	<-done
	pair.close()
	<-done
}

func (m *Manager) setErr(id string, running *runningForward, err error) {
	m.mu.Lock()
	if current, ok := m.forwards[id]; ok && current == running && running.running {
		running.err = err.Error()
	}
	m.mu.Unlock()
}

func copyConnection(done chan<- struct{}, dst io.Writer, src io.Reader) {
	_, _ = io.Copy(dst, src)
	done <- struct{}{}
}

func (p *connectionPair) setRemote(remote net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = remote.Close()
		return false
	}
	p.remote = remote
	return true
}

func (p *connectionPair) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	if p.local != nil {
		_ = p.local.Close()
	}
	if p.remote != nil {
		_ = p.remote.Close()
	}
}
