//go:build !windows

package syncengine

func pathHasReparsePoint(name string) bool {
	return false
}
