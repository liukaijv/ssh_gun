//go:build windows

package config

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protect(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	in := windows.DataBlob{
		Size: uint32(len(plaintext)),
		Data: &[]byte(plaintext)[0],
	}
	var out windows.DataBlob
	err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return "", fmt.Errorf("dpapi protect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := unsafe.Slice(out.Data, out.Size)
	cp := make([]byte, len(buf))
	copy(cp, buf)
	return base64.StdEncoding.EncodeToString(cp), nil
}

func unprotect(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("dpapi decode: %w", err)
	}
	in := windows.DataBlob{
		Size: uint32(len(raw)),
		Data: &raw[0],
	}
	var out windows.DataBlob
	err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return "", fmt.Errorf("dpapi unprotect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := unsafe.Slice(out.Data, out.Size)
	return string(buf), nil
}
