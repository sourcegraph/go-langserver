package util

import (
	"fmt"
	"go/ast"
	"log"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

func trimFilePrefix(s string) string {
	return strings.TrimPrefix(s, "file://")
}

// ConcatCommentGroups joins the resultant comment text from two CommentGroups with a newline.
func ConcatCommentGroups(a, b *ast.CommentGroup) string {
	a_text := a.Text()
	b_text := b.Text()
	if len(a_text) == 0 {
		return b_text
	} else if len(b_text) == 0 {
		return a_text
	} else {
		return a_text + "\n" + b_text
	}
}

// PathHasPrefix returns true if s is starts with the given prefix
func PathHasPrefix(s, prefix string) bool {
	s = virtualPath(trimFilePrefix(s))
	prefix = virtualPath(trimFilePrefix(prefix))
	if s == prefix {
		return true
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return s == prefix || strings.HasPrefix(s, prefix)
}

// PathTrimPrefix removes the prefix from s
func PathTrimPrefix(s, prefix string) string {
	s = virtualPath(trimFilePrefix(s))
	prefix = virtualPath(trimFilePrefix(prefix))
	if s == prefix {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.TrimPrefix(s, prefix)
}

// PathEqual returns true if both a and b are equal
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
	if path != "" {
		path = virtualPath(path)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	return lsp.DocumentURI("file://" + path)
}

// UriToPath converts given file URI to path
func UriToPath(uri lsp.DocumentURI) string {
	u, err := url.Parse(string(uri))
	if err != nil {
		return trimFilePrefix(string(uri))
	}
	return u.Path
}

// UriToRealPath converts the given file URI to the platform specific path
func UriToRealPath(uri lsp.DocumentURI) string {
	path := UriToPath(uri)
	return realPath(path)
}

// IsAbs returns true if the given path is absolute
func IsAbs(path string) bool {
	// Windows implementation accepts path-like and filepath-like arguments
	return strings.HasPrefix(path, "/") || filepath.IsAbs(path)
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
