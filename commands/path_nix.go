//go:build !windows
// +build !windows

package commands

// cleanRootPath is a no-op on every platform except Windows
func cleanRootPath(pattern string) string {
	return pattern
}

func osLineEnding() string {
	return "\n"
}

func isSyncRoot(path string) bool {
	return false
}

func isPlaceholderFile(path string) bool {
	return false
}
