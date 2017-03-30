// +build windows

package utils

import (
	"path/filepath"
	"strings"
)

func virtualPath(path string) string {
	// Windows implementation converts path to slashes and prefixes it with slash
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Also, on Windows paths are case-insensitive
	return strings.ToLower(path)
}

func IsAbs(path string) bool {
	// Windows implementation accepts path-like and filepath-like arguments
	return strings.HasPrefix(path, "/") || filepath.IsAbs(path)
}
