//go:build windows

package syncengine

import "golang.org/x/sys/windows"

const fileAttributeReparsePoint = 0x400

func pathHasReparsePoint(name string) bool {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&fileAttributeReparsePoint != 0
}
