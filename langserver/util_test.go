package langserver

import (
	"strings"
	"testing"
)

func TestPathAndUriConversion(t *testing.T) {
	tests := map[string]string{
		"/foo":           "file:///foo",
		"C:\\users\\bar": "file:///C%3A/users/bar",
		"/chip and dale": "file:///chip+and+dale",
	}
	for p, expected := range tests {
		uri := pathToUri(p)
		if uri != expected {
			t.Errorf("pathtouri: %s: expected %s, got %s", p, expected, uri)
		}
		path := uriToPath(uri)
		if strings.Replace(p, "\\", "/", -1) != path {
			t.Errorf("uritopath: %s: expected %s, got %s", uri, p, path)
		}

	}
}
