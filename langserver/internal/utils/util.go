package utils

import (
	"fmt"
	"log"
	"net/url"
	"runtime"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

func PathHasPrefix(s, prefix string) bool {
	prefix = virtualPath(prefix)
	var prefixSlash string
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefixSlash = prefix + "/"
	}
	s = virtualPath(s)
	return s == prefix || strings.HasPrefix(s, prefixSlash)
}

func PathTrimPrefix(s, prefix string) string {
	s = virtualPath(s)
	prefix = virtualPath(prefix)
	if s == prefix {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.TrimPrefix(s, prefix)
}

func PathEqual(a, b string) bool {
	return PathTrimPrefix(a, b) == ""
}

// IsVendorDir tells if the specified directory is a vendor directory.
func IsVendorDir(dir string) bool {
	return strings.HasPrefix(dir, "vendor/") || strings.Contains(dir, "/vendor/")
}

// IsURI tells if s denotes an URI
func IsURI(s lsp.DocumentURI) bool {
	return strings.HasPrefix(string(s), "file:///")
}

// PathToURI converts given absolute path to file URI
func PathToURI(path string) lsp.DocumentURI {
	path = virtualPath(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return lsp.DocumentURI("file://" + path)
}

// UriToPath converts given file URI to path
func UriToPath(uri lsp.DocumentURI) string {
	u, err := url.Parse(string(uri))
	if err != nil {
		return strings.TrimPrefix(string(uri), "file://")
	} else {
		return u.Path
	}
}

// Panicf takes the return value of recover() and outputs data to the log with
// the stack trace appended. Arguments are handled in the manner of
// fmt.Printf. Arguments should format to a string which identifies what the
// panic code was doing. Returns a non-nil error if it recovered from a panic.
func Panicf(r interface{}, format string, v ...interface{}) error {
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
