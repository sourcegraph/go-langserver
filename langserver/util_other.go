// +build !windows

package langserver

func vfsPath(path string) string {
	return normalizePath(path)
}

func normalizePath(path string) string {
	return path
}

func pathToURI(path string) string {
	return "file://" + path
}
