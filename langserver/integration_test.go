package langserver

import (
	"context"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

// TestIntegration_FileSystem tests using the server against the real
// OS file system, not a virtual file system. Then it tests it using
// the overlay (textDocument/didOpen unsaved file contents).
func TestIntegration_FileSystem(t *testing.T) {
	files := map[string]string{
		"A.go":    "package p; func A() {}",
		"B.go":    "package p; var _ = A",
		"P2/C.go": `package p2; import "test/p"; var _ = p.A`,
	}
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn) {
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
				Text: "package p; func A() int { return 0 }",
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
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn) {
		if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriJoin(rootURI, "A.go"),
				Text: "package p; func A() {}",
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
	integrationTest(t, files, func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn) {
		if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:  uriJoin(rootURI, "A.go"),
				Text: "package p;\n\n//   func A() {}\n",
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

func integrationTest(
	t *testing.T,
	files map[string]string,
	fn func(ctx context.Context, rootURI lsp.DocumentURI, conn *jsonrpc2.Conn),
) {
	tmpDir, err := ioutil.TempDir("", "langserver-go-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	orig := build.Default
	build.Default.GOPATH = filepath.Join(tmpDir, "gopath")
	build.Default.GOROOT = filepath.Join(tmpDir, "goroot")
	defer func() {
		build.Default = orig
	}()

	h := NewHandler(NewDefaultConfig())

	addr, done := startServer(t, h)
	defer done()
	conn := dialServer(t, addr)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Fatal("conn.Close:", err)
		}
	}()

	rootFSPath := filepath.Join(build.Default.GOPATH, "src/test/p")
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
	rootURI := util.PathToURI(rootFSPath)
	if err := conn.Call(ctx, "initialize", lsp.InitializeParams{RootURI: rootURI}, nil); err != nil {
		t.Fatal("initialize:", err)
	}

	fn(ctx, rootURI, conn)
}
