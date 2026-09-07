//go:build !windows

package singleinstance

import (
	"net"
	"time"
)

// Fixed loopback port for activate signaling on non-Windows.
const activateAddr = "127.0.0.1:37887"

func listenActivate() (net.Listener, error) {
	return net.Listen("tcp", activateAddr)
}

func dialActivate() (net.Conn, error) {
	return net.DialTimeout("tcp", activateAddr, 2*time.Second)
}
