// +build windows

package langserver

import (
	"path/filepath"
	"strings"
)

func virtualPath(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
