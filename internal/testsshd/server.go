package testsshd

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Config configures an in-process SSH+SFTP test server.
type Config struct {
	Root     string
	User     string
	Password string
	// PublicKeys, if non-empty, enables public key auth for matching keys.
	PublicKeys []ssh.PublicKey
	// Exec handles remote command execution. If nil, exec requests are rejected.
	Exec func(cmd string, stdin io.Reader, stdout, stderr io.Writer) error
}

// Server is a running in-process sshd.
type Server struct {
	ln     net.Listener
	cfg    Config
	sshCfg *ssh.ServerConfig
	wg     sync.WaitGroup
	quit   chan struct{}
}

// Start listens on 127.0.0.1:0 and serves SSH+SFTP until Close.
func Start(cfg Config) (*Server, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("root is required")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("user is required")
	}
	if _, err := os.Stat(cfg.Root); err != nil {
		return nil, fmt.Errorf("root: %w", err)
	}

	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == cfg.User && cfg.Password != "" && string(pass) == cfg.Password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() != cfg.User {
				return nil, fmt.Errorf("user rejected")
			}
			for _, allowed := range cfg.PublicKeys {
				if key.Type() == allowed.Type() && string(key.Marshal()) == string(allowed.Marshal()) {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("public key rejected")
		},
	}
	if cfg.Password == "" {
		sshCfg.PasswordCallback = nil
	}
	if len(cfg.PublicKeys) == 0 {
		sshCfg.PublicKeyCallback = nil
	}
	sshCfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &Server{
		ln:     ln,
		cfg:    cfg,
		sshCfg: sshCfg,
		quit:   make(chan struct{}),
	}
	s.wg.Add(1)
	go s.acceptLoop()
	return s, nil
}

// Addr returns the listen address (host:port).
func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Close stops the server.
func (s *Server) Close() error {
	close(s.quit)
	err := s.ln.Close()
	s.wg.Wait()
	return err
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(c)
		}(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshCfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go s.handleGlobalRequests(sshConn, reqs)

	for newChannel := range chans {
		switch newChannel.ChannelType() {
		case "session":
			ch, requests, err := newChannel.Accept()
			if err != nil {
				continue
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleSession(ch, requests)
			}()
		case "direct-tcpip":
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleDirectTCPIP(newChannel)
			}()
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown")
		}
	}
}

type tcpipForwardRequest struct {
	Addr string
	Port uint32
}

type forwardedTCPIPPayload struct {
	Addr       string
	Port       uint32
	OriginAddr string
	OriginPort uint32
}

func (s *Server) handleGlobalRequests(conn *ssh.ServerConn, reqs <-chan *ssh.Request) {
	forwards := make(map[string]net.Listener)
	defer func() {
		for _, ln := range forwards {
			_ = ln.Close()
		}
	}()

	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			var msg tcpipForwardRequest
			if err := ssh.Unmarshal(req.Payload, &msg); err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			bindHost := msg.Addr
			if bindHost == "" {
				bindHost = "0.0.0.0"
			}
			ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, fmt.Sprintf("%d", msg.Port)))
			if err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			boundPort := uint32(ln.Addr().(*net.TCPAddr).Port)
			key := net.JoinHostPort(msg.Addr, fmt.Sprintf("%d", boundPort))
			if msg.Port != 0 {
				key = net.JoinHostPort(msg.Addr, fmt.Sprintf("%d", msg.Port))
			}
			forwards[key] = ln

			var reply []byte
			if msg.Port == 0 {
				reply = ssh.Marshal(&struct{ Port uint32 }{boundPort})
			}
			if req.WantReply {
				_ = req.Reply(true, reply)
			}

			listenAddr := msg.Addr
			listenPort := boundPort
			if msg.Port != 0 {
				listenPort = msg.Port
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.serveRemoteForward(conn, ln, listenAddr, listenPort)
			}()

		case "cancel-tcpip-forward":
			var msg tcpipForwardRequest
			if err := ssh.Unmarshal(req.Payload, &msg); err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			key := net.JoinHostPort(msg.Addr, fmt.Sprintf("%d", msg.Port))
			if ln, ok := forwards[key]; ok {
				_ = ln.Close()
				delete(forwards, key)
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
			} else if req.WantReply {
				_ = req.Reply(false, nil)
			}

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) serveRemoteForward(conn *ssh.ServerConn, ln net.Listener, listenAddr string, listenPort uint32) {
	defer ln.Close()
	for {
		incoming, err := ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()

			originHost, originPortStr, _ := net.SplitHostPort(c.RemoteAddr().String())
			originPort := uint32(0)
			if p, err := strconv.ParseUint(originPortStr, 10, 32); err == nil {
				originPort = uint32(p)
			}
			payload := ssh.Marshal(&forwardedTCPIPPayload{
				Addr:       listenAddr,
				Port:       listenPort,
				OriginAddr: originHost,
				OriginPort: originPort,
			})
			ch, requests, err := conn.OpenChannel("forwarded-tcpip", payload)
			if err != nil {
				return
			}
			defer ch.Close()
			go ssh.DiscardRequests(requests)

			done := make(chan struct{}, 2)
			go func() {
				_, _ = io.Copy(ch, c)
				done <- struct{}{}
			}()
			go func() {
				_, _ = io.Copy(c, ch)
				done <- struct{}{}
			}()
			<-done
		}(incoming)
	}
}

func (s *Server) handleDirectTCPIP(newChannel ssh.NewChannel) {
	var request struct {
		DestHost   string
		DestPort   uint32
		OriginHost string
		OriginPort uint32
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &request); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "invalid direct-tcpip request")
		return
	}

	target := net.JoinHostPort(request.DestHost, fmt.Sprintf("%d", request.DestPort))
	remote, err := net.Dial("tcp", target)
	if err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer remote.Close()

	ch, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	go ssh.DiscardRequests(requests)

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, ch)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(ch, remote)
		done <- struct{}{}
	}()
	<-done
}

func (s *Server) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer ch.Close()
	for req := range requests {
		switch req.Type {
		case "subsystem":
			if string(req.Payload[4:]) == "sftp" {
				_ = req.Reply(true, nil)
				server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(s.cfg.Root))
				if err != nil {
					return
				}
				_ = server.Serve()
				_ = server.Close()
				return
			}
			_ = req.Reply(false, nil)
		case "exec":
			if s.cfg.Exec == nil {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			cmdLen := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
			cmd := string(req.Payload[4 : 4+cmdLen])
			err := s.cfg.Exec(cmd, ch, ch, ch.Stderr())
			status := struct{ Status uint32 }{0}
			if err != nil {
				status.Status = 1
				_, _ = fmt.Fprintf(ch.Stderr(), "%v\n", err)
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(&status))
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}
