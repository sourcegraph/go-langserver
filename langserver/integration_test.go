package langserver

import (
	"context"
	"encoding/json"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

// TestIntegration_FileSystem tests using the server against the real
// OS file system, not a virtual file system. Then it tests it using
// the overlay (textDocument/didOpen unsaved file contents).
func TestIntegration_FileSystem(t *testing.T) {
	files := map[string]string{
		"A.go":    "package p; func A() int { return 0 }",
		"B.go":    "package p; var _ = A",
		"P2/C.go": `package p2; import "test/p"; var _ = p.A`,
	}
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn, notifies chan *jsonrpc2.Request) {
		// Test some hovers using files on disk.
		cases := lspTestCases{
			wantHover: map[string]string{
				"A.go:1:17":    "func A()",
				"B.go:1:20":    "func A()",
				"P2/C.go:1:40": "func A()",
			},
		}
		lspTests(t, ctx, nil, conn, rootURI, cases)

		// Now mimic what happens when a file is edited but not yet
		// saved. It should re-typecheck using the unsaved file contents.
		if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriJoin(rootURI, "A.go"),
				Text: files["A.go"],
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didOpen:", err)
		}
		cases = lspTestCases{
			wantHover: map[string]string{
				"A.go:1:17":    "func A() int",
				"B.go:1:20":    "func A() int",
				"P2/C.go:1:40": "func A() int",
			},
		}
		lspTests(t, ctx, nil, conn, rootURI, cases)

		// Test incremental sync
		if err := conn.Call(ctx, "textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriJoin(rootURI, "A.go")},
				Version:                1,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				{
					Range: &lsp.Range{
						Start: lsp.Position{Line: 0, Character: 18},
						End:   lsp.Position{Line: 0, Character: 18},
					},
					RangeLength: 0,
					Text:        "i int",
				},
				{
					Range: &lsp.Range{
						Start: lsp.Position{Line: 0, Character: 25},
						End:   lsp.Position{Line: 0, Character: 29},
					},
					Text: "",
				},
				{
					Range: &lsp.Range{
						Start: lsp.Position{Line: 0, Character: 27},
						End:   lsp.Position{Line: 0, Character: 35},
					},
					Text: "A(i)",
				},
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didChange:", err)
		}
		cases = lspTestCases{
			wantHover: map[string]string{
				"A.go:1:28":    "func A(i int)",
				"B.go:1:20":    "func A(i int)",
				"P2/C.go:1:40": "func A(i int)",
			},
		}
		lspTests(t, ctx, nil, conn, rootURI, cases)
	})
}

func TestIntegration_FileSystem_Format(t *testing.T) {
	files := map[string]string{
		"A.go": "package p; func A() {}",
	}
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn, notifies chan *jsonrpc2.Request) {
		if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriJoin(rootURI, "A.go"),
				Text: files["A.go"],
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didOpen:", err)
		}

		// add the func argument
		if err := conn.Call(ctx, "textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriJoin(rootURI, "A.go")},
				Version:                1,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				toContentChange(toRange(0, 18, 0, 18), 0, "i int"),
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didChange:", err)
		}

		// expect the file to formatted with the following changes
		cases := lspTestCases{
			wantFormatting: map[string]map[string]string{
				"A.go": map[string]string{
					"0:0-1:0": "package p\n\nfunc A(i int) {}\n",
				},
			},
		}
		lspTests(t, ctx, nil, conn, rootURI, cases)
	})
}

func TestIntegration_FileSystem_Format2(t *testing.T) {
	files := map[string]string{
		"A.go": "package p;\n\n//   func A() {}\n",
	}
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn, notifies chan *jsonrpc2.Request) {
		if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriJoin(rootURI, "A.go"),
				Text: files["A.go"],
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didOpen:", err)
		}

		// remove the //
		if err := conn.Call(ctx, "textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriJoin(rootURI, "A.go")},
				Version:                1,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				toContentChange(toRange(2, 0, 2, 2), 2, ""),
			},
		}, nil); err != nil {
			t.Fatal("textDocument/didChange:", err)
		}

		// expect the file to formatted with the following changes
		cases := lspTestCases{
			wantFormatting: map[string]map[string]string{
				"A.go": map[string]string{
					"2:0-3:0": "func A() {}\n",
				},
			},
		}
		lspTests(t, ctx, nil, conn, rootURI, cases)
	})
}

func TestIntegration_FileSystem_Diagnostics(t *testing.T) {
	files := map[string]string{
		"A.go": strings.Join([]string{
			"package p",
			"",
			"func A1(i int) {}",
			"func A2(i int) {",
			"	A1(123)",
			"}",
		}, "\n"),
		"B.go": strings.Join([]string{
			"package p",
			"",
			"func B() {",
			"	A1(123)",
			"}",
		}, "\n"),
	}

	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn, notifies chan *jsonrpc2.Request) {
		uriA := uriJoin(rootURI, "A.go")
		uriB := uriJoin(rootURI, "B.go")

		call := callFn(ctx, t, conn)

		call("textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriA,
				Text: files["A.go"],
			},
		})
		call("textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriB,
				Text: files["B.go"],
			},
		})

		// remove "i int" from "func A1(i int)" in A.go
		call("textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriA},
				Version:                1,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				toContentChange(toRange(2, 8, 2, 13), 5, ""),
			},
		})
		call("textDocument/didSave", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriA},
				Version:                1,
			},
		})

		var params1 lsp.PublishDiagnosticsParams
		var params2 lsp.PublishDiagnosticsParams
		receiveNotification(t, notifies, &params1)
		receiveNotification(t, notifies, &params2)

		// expect at least one error in A.go and one in B.go
		actual := publishedDiagnosticsToMap(params1, params2)
		expected := map[lsp.DocumentURI]int{
			uriA: 1,
			uriB: 1,
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Expected diagnostics for %v but got diagnostics %v", expected, actual)
		}

		// fix B.go by removing the 123 from the "A1(123)" call
		call("textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriB},
				Version:                2,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				toContentChange(toRange(3, 4, 3, 7), 3, ""),
			},
		})
		call("textDocument/didSave", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriB},
				Version:                2,
			},
		})

		receiveNotification(t, notifies, &params1)
		receiveNotification(t, notifies, &params2)

		// expect at least one error in A.go and no diagnostics for B.go
		actual = publishedDiagnosticsToMap(params1, params2)
		expected = map[lsp.DocumentURI]int{
			uriA: 1,
			uriB: 0,
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Expected diagnostics for %v but got diagnostics %v", expected, actual)
		}

		// fix A.go by removing the 123 from the "A1(123)" call too
		call("textDocument/didChange", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriA},
				Version:                3,
			},
			ContentChanges: []lsp.TextDocumentContentChangeEvent{
				toContentChange(toRange(4, 4, 4, 7), 3, ""),
			},
		})
		call("textDocument/didSave", lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uriA},
				Version:                3,
			},
		})

		receiveNotification(t, notifies, &params1)

		// expect at no diagnostics for A.go and nothing for B.go
		actual = publishedDiagnosticsToMap(params1)
		expected = map[lsp.DocumentURI]int{
			uriA: 0,
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Expected diagnostics for %v but got diagnostics %v", expected, actual)
		}
	})
}

func integrationTest(
	t *testing.T,
	files map[string]string,
	fn func(context.Context, lsp.DocumentURI, *jsonrpc2.Conn, chan *jsonrpc2.Request),
) {
	tmpDir, err := ioutil.TempDir("", "langserver-go-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	GOPATH := filepath.Join(tmpDir, "gopath")
	GOROOT := filepath.Join(tmpDir, "goroot")

	rootFSPath := filepath.Join(GOPATH, "src/test/p")
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

	cfg := NewDefaultConfig()
	cfg.UseBinaryPkgCache = true
	h := NewHandler(cfg)

	addr, done := startServer(t, h)
	defer done()

	notifies := make(chan *jsonrpc2.Request, 1)
	conn := dialServer(t, addr, func(_ context.Context, _ *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		notifies <- req
		return nil, nil
	})
	defer func() {
		if err := conn.Close(); err != nil {
			t.Fatal("conn.Close:", err)
		}
	}()

	rootURI := util.PathToURI(rootFSPath)
	params := InitializeParams{
		InitializeParams: lsp.InitializeParams{
			RootURI: rootURI,
		},
		BuildContext: &InitializeBuildContextParams{
			GOOS:     build.Default.GOOS,
			GOARCH:   build.Default.GOARCH,
			GOPATH:   GOPATH,
			GOROOT:   GOROOT,
			Compiler: runtime.Compiler,
		},
	}

	ctx := context.Background()
	if err := conn.Call(ctx, "initialize", params, nil); err != nil {
		t.Fatal("initialize:", err)
	}

	fn(ctx, rootURI, conn, notifies)
}

func callFn(ctx context.Context, t *testing.T, conn *jsonrpc2.Conn) func(string, interface{}) {
	return func(m string, v interface{}) {
		if err := conn.Call(ctx, m, v, nil); err != nil {
			t.Fatal(m+":", err)
		}
	}
}

func receiveNotification(t *testing.T, ch chan *jsonrpc2.Request, v interface{}) {
	for {
		select {
		case n := <-ch:
			err := json.Unmarshal(*n.Params, v)
			if err != nil {
				t.Fatal(err)
			}
			return

		case <-time.After(time.Second * 10):
			t.Fatalf("Timeout while waiting for notification of type %T", v)
		}
	}
}

func publishedDiagnosticsToMap(diags ...lsp.PublishDiagnosticsParams) map[lsp.DocumentURI]int {
	d := map[lsp.DocumentURI]int{}
	for _, diag := range diags {
		d[diag.URI] = len(diag.Diagnostics)
	}
	return d
}
