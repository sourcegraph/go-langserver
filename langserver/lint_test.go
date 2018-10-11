package langserver

import (
	"context"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-lsp"
)

func TestParseLintLine(t *testing.T) {
	tests := []struct {
		l       string
		file    string
		line    int
		char    int
		message string
	}{
		{
			l:       "A.go:1: message contents",
			file:    "A.go",
			line:    0,
			char:    -1,
			message: "message contents",
		},
		{
			l:       "A.go:1:2 message contents",
			file:    "A.go",
			line:    0,
			char:    1,
			message: "message contents",
		},
		{
			l:       "A.go:1:2: message contents",
			file:    "A.go",
			line:    0,
			char:    1,
			message: "message contents",
		},
	}

	for _, test := range tests {
		file, line, char, message, err := parseLintResult(test.l)
		if err != nil {
			t.Errorf("unexpected error parsing %q: %v", test.l, err)
		}
		if file != test.file {
			t.Errorf("unexpected file parsing %q: want %q, have %q", test.l, test.file, file)
		}
		if line != test.line {
			t.Errorf("unexpected line parsing %q: want %d, have %d", test.l, test.line, line)
		}
		if char != test.char {
			t.Errorf("unexpected char parsing %q: want %d, have %d", test.l, test.char, char)
		}
		if message != test.message {
			t.Errorf("unexpected message parsing %q: want %q, have %q", test.l, test.message, message)
		}

	}
}

func TestLinterGolint(t *testing.T) {
	files := map[string]string{
		"A.go": strings.Join([]string{
			"package p",
			"",
			"func A(){}",
			"",
			"func AA(){}",
		}, "\n"),
		"B.go": strings.Join([]string{
			"package p",
			"",
			"func B(){}",
		}, "\n"),
		"C.go": strings.Join([]string{
			"package p",
			"",
			`import "test/p/sub"`,
			"",
			"// C is a function",
			"func C(){",
			"	sub.D()",
			"}",
		}, "\n"),
		"sub/D.go": strings.Join([]string{
			"package sub",
			"",
			"func D(){}",
		}, "\n"),
	}

	linterTest(t, files, func(ctx context.Context, bctx *build.Context, rootURI lsp.DocumentURI) {
		uriA := uriJoin(rootURI, "A.go")
		uriB := uriJoin(rootURI, "B.go")
		uriD := uriJoin(rootURI, "sub/D.go")

		l := &golint{}

		// skip tests if the golint command is not found
		err := l.IsInstalled(ctx, bctx)
		if err != nil {
			t.Skipf("lint command 'golint' not found: %s", err)
		}

		// Lint the package "test/p" and look for correct results
		actual, err := l.Lint(ctx, bctx, "test/p")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
		expected := diagnostics{
			util.UriToRealPath(uriA): []*lsp.Diagnostic{
				{
					Message:  "exported function A should have comment or be unexported",
					Severity: lsp.Warning,
					Source:   "golint",
					Range: lsp.Range{
						Start: lsp.Position{Line: 2, Character: 0},
						End:   lsp.Position{Line: 2, Character: 0},
					},
				},
				{
					Message:  "exported function AA should have comment or be unexported",
					Severity: lsp.Warning,
					Source:   "golint",
					Range: lsp.Range{
						Start: lsp.Position{Line: 4, Character: 0},
						End:   lsp.Position{Line: 4, Character: 0},
					},
				},
			},
			util.UriToRealPath(uriB): []*lsp.Diagnostic{
				{
					Message:  "exported function B should have comment or be unexported",
					Severity: lsp.Warning,
					Source:   "golint",
					Range: lsp.Range{
						Start: lsp.Position{Line: 2, Character: 0},
						End:   lsp.Position{Line: 2, Character: 0},
					},
				},
			},
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected diagnostics %v but got %v", actual, expected)
		}

		// Lint the package and subpackages "test/p/..." look for correct lint results
		actual, err = l.Lint(ctx, bctx, "test/p/...")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
		expected[util.UriToRealPath(uriD)] = []*lsp.Diagnostic{
			{
				Message:  "exported function D should have comment or be unexported",
				Severity: lsp.Warning,
				Source:   "golint",
				Range: lsp.Range{ // currently only supporting line level errors
					Start: lsp.Position{Line: 2, Character: 0},
					End:   lsp.Position{Line: 2, Character: 0},
				},
			},
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected diagnostics %v but got %v", actual, expected)
		}
	})
}

func linterTest(t *testing.T, files map[string]string, fn func(ctx context.Context, bctx *build.Context, rootURI lsp.DocumentURI)) {
	tmpDir, err := ioutil.TempDir("", "langserver-go-linter")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	GOPATH := filepath.Join(tmpDir, "gopath")
	GOROOT := filepath.Join(tmpDir, "goroot")
	rootFSPath := filepath.Join(GOPATH, "src/test/p")

	if err := os.MkdirAll(GOROOT, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootFSPath, 0700); err != nil {
		t.Fatal(err)
	}
	for filename, contents := range files {
		path := filepath.Join(rootFSPath, filename)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	bctx := &build.Context{
		GOPATH: GOPATH,
		GOROOT: GOROOT,
	}

	fn(ctx, bctx, util.PathToURI(rootFSPath))
}
