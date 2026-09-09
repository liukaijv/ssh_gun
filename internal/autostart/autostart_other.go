//go:build !windows

package autostart

import "errors"

// Enabled is always false outside Windows.
func Enabled() (bool, error) {
	return false, nil
}

// SetEnabled returns an error when enabling on unsupported platforms.
func SetEnabled(enabled bool) error {
	if !enabled {
		return nil
	}
	return errors.New("launch at login is only supported on Windows")
}
