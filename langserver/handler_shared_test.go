package langserver

import (
	"context"
	"testing"

	"golang.org/x/tools/go/buildutil"
)

type testCase struct {
	ImportPath      string
	FromDir         string
	WantPackageName string
	WantImportPath  string
}

func TestDefaultFindPackageFuncInGOPATH(t *testing.T) {
	bctx := buildutil.FakeContext(map[string]map[string]string{
		// local packages
		"/gopath/src/test": {
			"main.go": "package main",
		},
		"/gopath/src/test/foo": {
			"foo.go": "package foo",
		},
		"/gopath/src/test/bar": {
			"bar.go": "package bar",
		},
		// vendored packages
		"/gopath/src/test/vendor/baz": {
			"baz.go": "package baz",
		},
		// stdlib packages
		"/goroot/src/strings": {
			"strings.go": "package strings",
		},
		// other packages under gopath
		"/gopath/src/other": {
			"other.go": "package other",
		},
	})
	bctx.GOPATH = "/gopath"
	bctx.GOROOT = "/goroot"

	ctx := context.Background()

	tests := []testCase{
		// local packages
		testCase{"test/foo", "", "foo", "test/foo"},
		testCase{"test/foo", "/gopath/src/test", "foo", "test/foo"},
		testCase{"test/bar", "/gopath/src/test", "bar", "test/bar"},
		testCase{"test/bar", "/gopath/src/test/foo", "bar", "test/bar"},
		// vendored packages, fakeContext now doesn't support vendor dir, :(
		// testCase{"baz", "/home/go/test", "baz", "test/vendor/baz"},
		// testCase{"baz", "/home/go/test/foo", "baz", "test/vendor/baz"},
		// stdlib packages
		testCase{"strings", "/gopath/src/test", "strings", "strings"},
		testCase{"strings", "/gopath/src/test/foo", "strings", "strings"},
		// other packages
		testCase{"other", "/gopath/src/test", "other", "other"},
		testCase{"other", "/gopath/src/test/foo", "other", "other"},
	}

	for _, test := range tests {
		pkg, err := defaultFindPackageFunc(ctx, bctx, test.ImportPath, test.FromDir, "/gopath/src/test", 0)
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Name != test.WantPackageName {
			t.Errorf("import %s from %s: got pkg name %q, want %q", test.ImportPath, test.FromDir, pkg.Name, test.WantPackageName)
		}
		if pkg.ImportPath != test.WantImportPath {
			t.Errorf("import %s from %s: got import path %q, want %q", test.ImportPath, test.FromDir, pkg.ImportPath, test.WantImportPath)
		}
	}
}

func TestDefaultFindPackageFuncOutGOPATH(t *testing.T) {
	bctx := buildutil.FakeContext(map[string]map[string]string{
		// local packages
		"/home/go/test": {
			"main.go": "package main",
		},
		"/home/go/test/foo": {
			"foo.go": "package foo",
		},
		"/home/go/test/bar": {
			"bar.go": "package bar",
		},
		// vendored packages
		"/home/go/test/vendor/baz": {
			"baz.go": "package baz",
		},
		// stdlib packages
		"/goroot/src/strings": {
			"strings.go": "package strings",
		},
		// other packages under gopath
		"/gopath/src/other": {
			"other.go": "package other",
		},
	})
	bctx.GOPATH = "/gopath"
	bctx.GOROOT = "/goroot"

	ctx := context.Background()

	tests := []testCase{
		// local packages
		testCase{"test/foo", "", "foo", "test/foo"},
		testCase{"test/foo", "/home/go/test", "foo", "test/foo"},
		testCase{"test/bar", "/home/go/test", "bar", "test/bar"},
		testCase{"test/bar", "/home/go/test/foo", "bar", "test/bar"},
		// vendored packages
		testCase{"baz", "/home/go/test", "baz", "test/vendor/baz"},
		testCase{"baz", "/home/go/test/foo", "baz", "test/vendor/baz"},
		// stdlib packages
		testCase{"strings", "/home/go/test", "strings", "strings"},
		testCase{"strings", "/home/go/test/foo", "strings", "strings"},
		// other packages
		testCase{"other", "/home/go/test", "other", "other"},
		testCase{"other", "/home/go/test/foo", "other", "other"},
	}

	for _, test := range tests {
		pkg, err := defaultFindPackageFunc(ctx, bctx, test.ImportPath, test.FromDir, "/home/go/test", 0)
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Name != test.WantPackageName {
			t.Errorf("import %s from %s: got pkg name %q, want %q", test.ImportPath, test.FromDir, pkg.Name, test.WantPackageName)
		}
		if pkg.ImportPath != test.WantImportPath {
			t.Errorf("import %s from %s: got import path %q, want %q", test.ImportPath, test.FromDir, pkg.ImportPath, test.WantImportPath)
		}
	}
}
