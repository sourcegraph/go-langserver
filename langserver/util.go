package langserver

import (
	"net/url"
	"strings"
)

func PathHasPrefix(s, prefix string) bool {
	s = normalizePath(s)
	prefixSlash := normalizePath(prefix)
	if prefixSlash != "" && !strings.HasSuffix(prefixSlash, "/") {
		prefixSlash += "/"
	}
	return s == prefix || strings.HasPrefix(s, prefixSlash)
}

func PathTrimPrefix(s, prefix string) string {
	s = normalizePath(s)
	prefix = normalizePath(prefix)
	if s == prefix {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.TrimPrefix(s, prefix)
}

func pathEqual(a, b string) bool {
	return PathTrimPrefix(a, b) == ""
}

// IsVendorDir tells if the specified directory is a vendor directory.
func IsVendorDir(dir string) bool {
	dir = normalizePath(dir)
	return strings.HasPrefix(dir, "vendor/") || strings.Contains(dir, "/vendor/")
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		path := strings.TrimPrefix(uri, "file://")
		// On Windows, VS Code sends "file:///c%3A/..."
		if unescaped, err := url.QueryUnescape(path); err == nil {
			return unescaped
		}
		return path
	}
	return uri
}
