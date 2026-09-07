//go:build plan9

package singleinstance

import (
	"errors"
	"os"
)

func flockExclusive(f *os.File) error {
	return errors.New("flock not supported on plan9")
}

func funlock(f *os.File) error { return nil }
