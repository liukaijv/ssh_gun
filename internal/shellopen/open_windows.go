//go:build windows

package shellopen

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func openPath(path string) error {
	// ShellExecute avoids CREATE_NO_WINDOW issues with explorer.exe and
	// does not flash a console window.
	return shellExecute(path, "")
}

func revealPath(path string) error {
	return shellExecute("explorer.exe", "/select,"+path)
}

func shellExecute(file, params string) error {
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return err
		}
	}
	err = windows.ShellExecute(0, nil, filePtr, paramsPtr, nil, windows.SW_SHOWNORMAL)
	if err != nil {
		return fmt.Errorf("ShellExecute: %w", err)
	}
	return nil
}
