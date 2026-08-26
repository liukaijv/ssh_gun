package sshclient

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/config"
)

// Pool reuses SSH clients keyed by server ID.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*ssh.Client
}

func NewPool() *Pool {
	return &Pool{clients: make(map[string]*ssh.Client)}
}

func (p *Pool) Get(ctx context.Context, srv config.Server) (*ssh.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[srv.ID]; ok {
		sess, err := c.NewSession()
		if err == nil {
			_ = sess.Close()
			return c, nil
		}
		_ = c.Close()
		delete(p.clients, srv.ID)
	}
	c, err := Dial(ctx, srv)
	if err != nil {
		return nil, err
	}
	p.clients[srv.ID] = c
	return c, nil
}

func (p *Pool) Invalidate(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[id]; ok {
		_ = c.Close()
		delete(p.clients, id)
	}
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.clients {
		_ = c.Close()
		delete(p.clients, id)
	}
}

// Dial opens a new SSH connection for the given server config.
func Dial(ctx context.Context, srv config.Server) (*ssh.Client, error) {
	auth, err := authMethods(srv)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback(srv),
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(srv.Host, fmt.Sprintf("%d", srv.Port))
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func authMethods(srv config.Server) ([]ssh.AuthMethod, error) {
	switch srv.AuthType {
	case "password", "":
		return []ssh.AuthMethod{ssh.Password(srv.Password)}, nil
	case "key":
		key, err := os.ReadFile(srv.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		var signer ssh.Signer
		if srv.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(srv.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("unknown auth_type %q", srv.AuthType)
	}
}

func hostKeyCallback(srv config.Server) ssh.HostKeyCallback {
	// accept-new / insecure for now; strict known_hosts can be layered later.
	_ = srv.HostKeyPolicy
	return ssh.InsecureIgnoreHostKey()
}

// TestConnection dials once and reports success.
func TestConnection(ctx context.Context, srv config.Server) (bool, string, error) {
	c, err := Dial(ctx, srv)
	if err != nil {
		return false, err.Error(), nil
	}
	_ = c.Close()
	return true, "ok", nil
}

// Exec runs a remote command and returns combined stdout.
func Exec(ctx context.Context, client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	done := make(chan error, 1)
	var out []byte
	go func() {
		var e error
		out, e = session.CombinedOutput(cmd)
		done <- e
	}()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", ctx.Err()
	case err := <-done:
		return string(out), err
	}
}

// NewSFTP opens an SFTP subsystem over an existing SSH connection.
func NewSFTP(client *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(client)
}
