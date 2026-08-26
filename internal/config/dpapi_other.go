//go:build !windows

package config

// Non-Windows fallback for tests on other platforms — NOT used in production builds.
func protect(plaintext string) (string, error) {
	return "plain:" + plaintext, nil
}

func unprotect(encoded string) (string, error) {
	const prefix = "plain:"
	if len(encoded) >= len(prefix) && encoded[:len(prefix)] == prefix {
		return encoded[len(prefix):], nil
	}
	return encoded, nil
}
