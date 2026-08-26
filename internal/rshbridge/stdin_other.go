//go:build !windows

package rshbridge

import (
	"io"
	"os"
)

func bridgeInput() (io.Reader, func(), error) {
	return os.Stdin, func() {}, nil
}
