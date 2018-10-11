package langserver

import (
	"context"
	"encoding/json"
	"fmt"
	"go/build"
	"go/token"
	"path"
	"reflect"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"

	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
)

var loaderCases = map[string]struct {
	fs map[string]string
}{
	"standalone": {fs: map[string]string{"/src/p/f.go": `package p; func F() {}`}},
	"imports net/http": {
		fs: map[string]string{"/src/p/f.go": `package p; import "net/http"; var _ = http.Get`},
	},
	"build-tagged different package in dir": {
		fs: map[string]string{
			"/src/p/f.go": `package p`,
			"/src/p/main.go": `// +build ignore

package main`,
		},
	},
	"multiple packages in dir": {
		fs: map[string]string{
			"/src/p/f.go":    `package p`,
			"/src/p/main.go": `package main`,
		},
	},
}

func TestLoader(t *testing.T) {
	ctx := context.Background()
	for label, tc := range loaderCases {
		t.Run(label, func(t *testing.T) {
			fset, bctx, bpkg := setUpLoaderTest(tc.fs)
			p, _, err := typecheck(ctx, fset, bctx, bpkg, defaultFindPackageFunc, "/src/p")
			if err != nil {
				t.Error(err)
			} else if len(p.Created) == 0 {
				t.Error("Expected to loader to create a package")
			} else if len(p.Created[0].Files) == 0 {
				t.Error("did not load any files")
			}
		})
	}
}

// BenchmarkLoader measures the performance of loading and
// typechecking.
//
// Run it with:
//
//   go test ./langserver -bench Loader -benchmem
func BenchmarkLoader(b *testing.B) {
	ctx := context.Background()
	for label, tc := range loaderCases {
		b.Run(label, func(b *testing.B) {
			fset, bctx, bpkg := setUpLoaderTest(tc.fs)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := typecheck(ctx, fset, bctx, bpkg, defaultFindPackageFunc, "/src/p"); err != nil {
					b.Error(err)
				}
			}
		})
	}
}

func TestLoaderDiagnostics(t *testing.T) {
	m := func(s string) diagnostics {
		var d diagnostics
		err := json.Unmarshal([]byte(s), &d)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cases := []struct {
		Name string
		FS   map[string]string
		// Want is a slice to cater for slight changes in error messages
		// across go versions.
		Want []diagnostics
	}{
		{
			Name: "none",
			FS:   map[string]string{"/src/p/f.go": `package p; func F() {}`},
		},
		{
			Name: "malformed",
			FS:   map[string]string{"/src/p/f.go": `234ljsdfjb2@#%$`},
			Want: []diagnostics{
				m(`{"/src/p/f.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"severity":1,"source":"go","message":"expected 'package', found 'INT' 234 (and 4 more errors)"}]}`),
				m(`{"/src/p/f.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"severity":1,"source":"go","message":"expected 'package', found 234 (and 4 more errors)"}]}`),
			},
		},
		{
			Name: "undeclared",
			FS:   map[string]string{"/src/p/f.go": `package p; var _ = http.Get`},
			Want: []diagnostics{
				m(`{"/src/p/f.go":[{"range":{"start":{"line":0,"character":19},"end":{"line":0,"character":23}},"severity":1,"source":"go","message":"undeclared name: http"}]}`),
			},
		},
	}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			fset, bctx, bpkg := setUpLoaderTest(tc.FS)
			_, diag, err := typecheck(ctx, fset, bctx, bpkg, defaultFindPackageFunc, "/src/p")
			if err != nil {
				t.Error(err)
			}
			found := false
			for _, want := range tc.Want {
				found = found || reflect.DeepEqual(diag, want)
			}
			if found {
				return
			}
			var want diagnostics
			if len(tc.Want) > 0 {
				want = tc.Want[0]
			}
			if !reflect.DeepEqual(diag, want) {
				got, _ := json.Marshal(diag)
				wantS, _ := json.Marshal(want)
				t.Errorf("got %s\nwant %s", string(got), string(wantS))
			}
		})
	}
}

func setUpLoaderTest(fs map[string]string) (*token.FileSet, *build.Context, *build.Package) {
	h := LangHandler{HandlerShared: new(HandlerShared)}
	if err := h.reset(&InitializeParams{
		InitializeParams:     lsp.InitializeParams{RootURI: "file:///src/p"},
		NoOSFileSystemAccess: true,
		BuildContext: &InitializeBuildContextParams{
			GOPATH: "/",
		},
	}); err != nil {
		panic(err)
	}
	_, ctx := opentracing.StartSpanFromContext(context.Background(), "loadertest")
	for filename, contents := range fs {
		r := &jsonrpc2.Request{Method: "textDocument/didOpen"}
		r.SetParams(&lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
			URI:  util.PathToURI(filename),
			Text: contents,
		}})
		_, _, err := h.handleFileSystemRequest(ctx, r)
		if err != nil {
			panic(err)
		}
	}
	bctx := h.BuildContext(context.Background())
	bctx.GOPATH = "/"
	goFiles := make([]string, 0, len(fs))
	for n := range fs {
		goFiles = append(goFiles, path.Base(n))
	}
	return token.NewFileSet(), bctx, &build.Package{ImportPath: "p", Dir: "/src/p", GoFiles: goFiles}
}

func TestBuildPackageForNamedFileInMultiPackageDir(t *testing.T) {
	tests := map[string]struct {
		bpkg *build.Package
		m    *build.MultiplePackageError
		want map[string]*build.Package // filename -> expected pkg
	}{
		"a and b": {
			bpkg: &build.Package{
				GoFiles:      []string{"a.go", "b.go"},
				TestGoFiles:  []string{"a_test.go", "b_test.go"},
				XTestGoFiles: []string{"xa_test.go", "xb_test.go"},
			},
			m: &build.MultiplePackageError{
				Packages: []string{"a", "a", "b", "b", "a_test", "b_test"},
				Files:    []string{"a.go", "a_test.go", "b.go", "b_test.go", "xa_test.go", "xb_test.go"},
			},
			want: map[string]*build.Package{
				"a.go":       {Name: "a", GoFiles: []string{"a.go"}, TestGoFiles: []string{"a_test.go"}, XTestGoFiles: []string{"xa_test.go"}},
				"a_test.go":  {Name: "a", GoFiles: []string{"a.go"}, TestGoFiles: []string{"a_test.go"}, XTestGoFiles: []string{"xa_test.go"}},
				"xa_test.go": {Name: "a_test", GoFiles: []string{"a.go"}, TestGoFiles: []string{"a_test.go"}, XTestGoFiles: []string{"xa_test.go"}},
				"b.go":       {Name: "b", GoFiles: []string{"b.go"}, TestGoFiles: []string{"b_test.go"}, XTestGoFiles: []string{"xb_test.go"}},
				"b_test.go":  {Name: "b", GoFiles: []string{"b.go"}, TestGoFiles: []string{"b_test.go"}, XTestGoFiles: []string{"xb_test.go"}},
				"xb_test.go": {Name: "b_test", GoFiles: []string{"b.go"}, TestGoFiles: []string{"b_test.go"}, XTestGoFiles: []string{"xb_test.go"}},
			},
		},
		"two main packages": {
			// TODO(sqs): If the package name is "main", and there are
			// multiple main packages that are separate programs (and,
			// e.g., expected to be run directly run `go run main1.go
			// main2.go`), then it will break because it will try to
			// compile them all together. There's no good way to handle
			// that case that I can think of, other than with heuristics.
			bpkg: &build.Package{
				GoFiles:     []string{"main1.go", "main2.go"},
				TestGoFiles: []string{"main_test.go"},
			},
			m: &build.MultiplePackageError{
				Packages: []string{"main", "main", "main"},
				Files:    []string{"main1.go", "main2.go", "main_test.go"},
			},
			want: map[string]*build.Package{
				"main1.go": {Name: "main", GoFiles: []string{"main1.go", "main2.go"}, TestGoFiles: []string{"main_test.go"}},
				"main2.go": {Name: "main", GoFiles: []string{"main1.go", "main2.go"}, TestGoFiles: []string{"main_test.go"}},
			},
		},
		"main with test": {
			bpkg: &build.Package{
				GoFiles:     []string{"a.go", "main.go"},
				TestGoFiles: []string{"main_test.go"},
			},
			m: &build.MultiplePackageError{
				Packages: []string{"a", "main", "main"},
				Files:    []string{"a.go", "main.go", "main_test.go"},
			},
			want: map[string]*build.Package{
				"a.go":         {Name: "a", GoFiles: []string{"a.go"}},
				"main.go":      {Name: "main", GoFiles: []string{"main.go"}, TestGoFiles: []string{"main_test.go"}},
				"main_test.go": {Name: "main", GoFiles: []string{"main.go"}, TestGoFiles: []string{"main_test.go"}},
			},
		},
	}
	for label, test := range tests {
		t.Run(label, func(t *testing.T) {
			for filename, want := range test.want {
				t.Run(filename, func(t *testing.T) {
					bpkg, err := buildPackageForNamedFileInMultiPackageDir(test.bpkg, test.m, filename)
					if err != nil {
						t.Fatalf("%s: %s: %s", label, filename, err)
					}
					if !reflect.DeepEqual(bpkg, want) {
						printPkg := func(p *build.Package) string {
							return fmt.Sprintf("build.Package{Name:%s GoFiles:%v TestGoFiles:%v XTestGoFiles:%v}", p.Name, p.GoFiles, p.TestGoFiles, p.XTestGoFiles)
						}
						t.Errorf("%s: %s:\n got %s\nwant %s", label, filename, printPkg(bpkg), printPkg(want))
					}
				})
			}
		})
	}
}
