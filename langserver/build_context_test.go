package langserver

import (
	"testing"

	"golang.org/x/tools/go/buildutil"
)

func TestContainingPackageInGOPATH(t *testing.T) {
	bctx := buildutil.FakeContext(map[string]map[string]string{
		"p": {
			"a.go":      "package p",
			"a_test.go": "package p_test",
		},
	})
	bctx.GOPATH = "/go"

	tests := map[string]string{
		"/go/src/p/a.go":      "p",
		"/go/src/p/a_test.go": "p_test",
	}
	for file, wantPkgName := range tests {
		pkg, err := ContainingPackage(bctx, file, "")
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Name != wantPkgName {
			t.Errorf("%s: got pkg name %q, want %q", file, pkg.Name, wantPkgName)
		}
	}
}

func TestContainingPackageOutGOPATH(t *testing.T) {
	bctx := buildutil.FakeContext(map[string]map[string]string{
		"/home/me/p": {
			"a.go":      "package p",
			"a_test.go": "package p_test",
		},
	})
	bctx.GOPATH = "/go"

	tests := map[string]string{
		"/home/me/p/a.go":      "p",
		"/home/me/p/a_test.go": "p_test",
	}
	for file, wantPkgName := range tests {
		pkg, err := ContainingPackage(bctx, file, "/home/me/p")
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Name != wantPkgName {
			t.Errorf("%s: got pkg name %q, want %q", file, pkg.Name, wantPkgName)
		}
	}
}
