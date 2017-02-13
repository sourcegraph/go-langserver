// +build !windows

package utils

import (
	"path/filepath"
)

func virtualPath(path string) string {
	// non-Windows implementation does nothing
	return path
}

func IsAbs(path string) bool {
	// non-Windows implementation uses filepath method
	return filepath.IsAbs(path)
}
