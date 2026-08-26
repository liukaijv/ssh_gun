//go:build windows

package rshbridge

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func bridgeInput() (io.Reader, func(), error) {
	input, err := duplicateInput(os.Stdin)
	if err != nil {
		return nil, func() {}, err
	}
	return input, func() { _ = input.Close() }, nil
}

func duplicateInput(input *os.File) (*os.File, error) {
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		process,
		windows.Handle(input.Fd()),
		process,
		&duplicate,
		0,
		false,
		windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return nil, fmt.Errorf("duplicate standard input handle: %w", err)
	}
	file := os.NewFile(uintptr(duplicate), "rsync-stdin")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return nil, fmt.Errorf("wrap duplicated standard input handle")
	}
	return file, nil
}
