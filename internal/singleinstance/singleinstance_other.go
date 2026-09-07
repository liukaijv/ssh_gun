//go:build !windows

package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type otherGuard struct {
	lockFile *os.File
	server   *activateServer
}

func acquireImpl() (guardImpl, error) {
	dir := os.TempDir()
	path := filepath.Join(dir, "feisuo-ssh-gun.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, ErrAlreadyRunning
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	return &otherGuard{lockFile: f}, nil
}

func (g *otherGuard) listen(onActivate func()) error {
	server, err := startActivateServer(onActivate)
	if err != nil {
		return err
	}
	g.server = server
	return nil
}

func (g *otherGuard) close() {
	if g.server != nil {
		g.server.close()
		g.server = nil
	}
	if g.lockFile != nil {
		_ = funlock(g.lockFile)
		name := g.lockFile.Name()
		_ = g.lockFile.Close()
		_ = os.Remove(name)
		g.lockFile = nil
	}
}
