//go:build !windows

package shellopen

func openPath(path string) error {
	return openPathViaCommand(path)
}

func revealPath(path string) error {
	return revealPathViaCommand(path)
}
