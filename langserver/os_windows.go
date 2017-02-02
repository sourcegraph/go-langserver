// +build windows

package langserver

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
	return path
}

func isAbs(path string) bool {
	// Windows implementation accepts path-like and filepath-like arguments
	return strings.HasPrefix(path, "/") || filepath.IsAbs(path)
}