package forward

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	socks5Version      = 0x05
	socks5AuthNone     = 0x00
	socks5NoAcceptable = 0xFF

	socks5CmdConnect      = 0x01
	socks5CmdUDPAssociate = 0x03

	socks5ATypIPv4   = 0x01
	socks5ATypDomain = 0x03
	socks5ATypIPv6   = 0x04

	socks5RepSuccess             = 0x00
	socks5RepCommandNotSupported = 0x07
)

// ServeSOCKS5Handshake negotiates SOCKS5 with no authentication.
func ServeSOCKS5Handshake(conn net.Conn) error {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return err
	}
	if hdr[0] != socks5Version {
		return fmt.Errorf("unsupported socks version %d", hdr[0])
	}
	nMethods := int(hdr[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	ok := false
	for _, m := range methods {
		if m == socks5AuthNone {
			ok = true
			break
		}
	}
	if !ok {
		_, _ = conn.Write([]byte{socks5Version, socks5NoAcceptable})
		return fmt.Errorf("no acceptable auth method")
	}
	_, err := conn.Write([]byte{socks5Version, socks5AuthNone})
	return err
}

// ServeSOCKS5Connect reads a CONNECT request, replies success, and returns the target address.
func ServeSOCKS5Connect(conn net.Conn) (string, error) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return "", err
	}
	if hdr[0] != socks5Version {
		return "", fmt.Errorf("unsupported socks version %d", hdr[0])
	}
	cmd := hdr[1]
	atyp := hdr[3]

	host, err := readSOCKS5Addr(conn, atyp)
	if err != nil {
		return "", err
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	if cmd != socks5CmdConnect {
		_ = writeSOCKS5Reply(conn, socks5RepCommandNotSupported)
		return "", fmt.Errorf("unsupported socks command %d", cmd)
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	if err := writeSOCKS5Reply(conn, socks5RepSuccess); err != nil {
		return "", err
	}
	return addr, nil
}

func readSOCKS5Addr(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case socks5ATypIPv4:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case socks5ATypIPv6:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case socks5ATypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return "", err
		}
		buf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	default:
		return "", fmt.Errorf("unsupported address type %d", atyp)
	}
}

func writeSOCKS5Reply(conn net.Conn, rep byte) error {
	// VER REP RSV ATYP BND.ADDR(IPv4 0.0.0.0) BND.PORT(0)
	_, err := conn.Write([]byte{
		socks5Version, rep, 0x00, socks5ATypIPv4,
		0, 0, 0, 0,
		0, 0,
	})
	return err
}
