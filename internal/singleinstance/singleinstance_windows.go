//go:build windows

package singleinstance

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

const mutexName = `Local\FeisuoSSHGunSingleInstance`

type windowsGuard struct {
	mutex  windows.Handle
	server *activateServer
}

func acquireImpl() (guardImpl, error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, err
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if mutex == 0 {
		if err != nil {
			return nil, fmt.Errorf("create mutex: %w", err)
		}
		return nil, fmt.Errorf("create mutex: invalid handle")
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(mutex)
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		_ = windows.CloseHandle(mutex)
		return nil, fmt.Errorf("create mutex: %w", err)
	}
	return &windowsGuard{mutex: mutex}, nil
}

func (g *windowsGuard) listen(onActivate func()) error {
	server, err := startActivateServer(onActivate)
	if err != nil {
		return err
	}
	g.server = server
	return nil
}

func (g *windowsGuard) close() {
	if g.server != nil {
		g.server.close()
		g.server = nil
	}
	if g.mutex != 0 {
		_ = windows.CloseHandle(g.mutex)
		g.mutex = 0
	}
}
