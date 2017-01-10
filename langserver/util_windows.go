// +build windows

package langserver

import (
	"net/url"
	"path/filepath"
	"strings"
)

func normalizePath(path string) string {
	return strings.ToLower(filepath.ToSlash(path))
}

func pathToURI(path string) string {
	prefix := "file://"
	if filepath.IsAbs(path) {
		prefix += "/"
		return prefix + url.QueryEscape(filepath.ToSlash(path))
	}
	return prefix + filepath.ToSlash(path)
}
