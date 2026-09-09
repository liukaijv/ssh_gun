//go:build windows

package autostart

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// openRunKey opens the HKCU Run key (or a test override).
var openRunKey = func(access uint32) (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, runKeyPath, access)
}

// Enabled reports whether Feisuo is registered to launch at login.
func Enabled() (bool, error) {
	key, err := openRunKey(registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer key.Close()
	_, _, err = key.GetStringValue(ValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetEnabled adds or removes the HKCU Run entry for Feisuo.
func SetEnabled(enabled bool) error {
	if !enabled {
		key, err := openRunKey(registry.SET_VALUE)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				return nil
			}
			return err
		}
		defer key.Close()
		err = key.DeleteValue(ValueName)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("remove launch-at-login: %w", err)
		}
		return nil
	}

	cmd, err := executableCommand()
	if err != nil {
		return err
	}
	key, err := openRunKey(registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue(ValueName, cmd); err != nil {
		return fmt.Errorf("set launch-at-login: %w", err)
	}
	return nil
}
