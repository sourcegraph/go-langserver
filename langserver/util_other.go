// +build !windows

package langserver

func normalizePath(path string) string {
	return path
}

func pathToURI(path string) string {
	return "file://" + path
}
