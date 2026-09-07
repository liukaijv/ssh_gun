//go:build windows

package singleinstance

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

const activatePipe = `\\.\pipe\feisuo-ssh-gun-activate`

func listenActivate() (net.Listener, error) {
	return winio.ListenPipe(activatePipe, nil)
}

func dialActivate() (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(activatePipe, &timeout)
}
