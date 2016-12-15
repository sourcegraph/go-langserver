// +build windows

package langserver

import (
	"net/url"
	"path/filepath"
	"strings"
)

func vfsPath(path string) string {
	path = normalizePath(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

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
