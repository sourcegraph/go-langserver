// +build windows

package langserver

import (
	"path/filepath"
)

func virtualPath(path string) string {
	return filepath.ToSlash(path)
}
