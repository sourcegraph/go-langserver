// +build !windows

package langserver

import (
	"path/filepath"
)

func virtualPath(path string) string {
	// non-Windows implementation does nothing
	return path
}

func isAbs(path string) bool {
	// non-Windows implementation uses filepath method
	return filepath.IsAbs(path)
}
