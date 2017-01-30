package langserver

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"strings"
)

func PathHasPrefix(s, prefix string) bool {
	var prefixSlash string
	if prefix != "" && !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefixSlash = prefix + string(os.PathSeparator)
	}
	return s == prefix || strings.HasPrefix(s, prefixSlash)
}

func PathTrimPrefix(s, prefix string) string {
	if s == prefix {
		return ""
	}
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}
	return strings.TrimPrefix(s, prefix)
}

func pathEqual(a, b string) bool {
	return PathTrimPrefix(a, b) == ""
}

// IsVendorDir tells if the specified directory is a vendor directory.
func IsVendorDir(dir string) bool {
	return strings.HasPrefix(dir, "vendor/") || strings.Contains(dir, "/vendor/")
}

// isUri tells if s denotes an URI
func isUri(s string) bool {
	return strings.HasPrefix(s, "file://")
}

// pathToUri converts given path to file URI
func pathToUri(path string) string {
	prefix := "file://"
	if !strings.HasPrefix(path, "/") {
		// On Windows there should be triple slash
		prefix += "/"
	}
	path = slashPath(path)
	// encoding URI components
	// TODO: wait for net/url: PathEscape, PathUnescape
	// see https://github.com/golang/go/commit/7e2bf952a905f16a17099970392ea17545cdd193
	components := strings.Split(path, "/")
	for i, _ := range components {
		components[i] = url.QueryEscape(components[i])
	}
	return prefix + strings.Join(components, "/")
}

// uriToPath converts given file URI to path
func uriToPath(uri string) string {
	if isUri(uri) {
		path := strings.TrimPrefix(uri, "file://")
		// On Windows, VS Code sends "file:///c%3A/..."
		if unescaped, err := url.QueryUnescape(path); err == nil {
			path = unescaped
		}
		// checking if we have a Windows-style URL such as file:///C:/
		// if so, removing leading slash
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		return path
	}
	return uri
}

// slashPath converts path to use slashes as component separators. Doesn't affect Unix-style paths,
// only Windows-style paths (rune ':' ...) being converted to use slashes
func slashPath(p string) string {
	if len(p) > 2 && p[1] == ':' {
		return strings.Replace(p, "\\", "/", -1)
	}
	return p
}

// panicf takes the return value of recover() and outputs data to the log with
// the stack trace appended. Arguments are handled in the manner of
// fmt.Printf. Arguments should format to a string which identifies what the
// panic code was doing. Returns a non-nil error if it recovered from a panic.
func panicf(r interface{}, format string, v ...interface{}) error {
	if r != nil {
		// Same as net/http
		const size = 64 << 10
		buf := make([]byte, size)
		buf = buf[:runtime.Stack(buf, false)]
		id := fmt.Sprintf(format, v...)
		log.Printf("panic serving %s: %v\n%s", id, r, string(buf))
		return fmt.Errorf("unexpected panic: %v", r)
	}
	return nil
}
