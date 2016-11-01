package langserver

import (
	"context"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

// TestIntegration_FileSystem tests using the server against the real
// OS file system, not a virtual file system. Then it tests it using
// the overlay (textDocument/didOpen unsaved file contents).
func TestIntegration_FileSystem(t *testing.T) {
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

	h := NewHandler()

	addr, done := startServer(t, h)
	defer done()
	conn := dialServer(t, addr)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Fatal("conn.Close:", err)
		}
	}()

	rootPath := filepath.Join(build.Default.GOPATH, "src/test/p")
	if err := os.MkdirAll(rootPath, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.go":    "package p; func A() {}",
		"b.go":    "package p; var _ = A",
		"p2/c.go": `package p2; import "test/p"; var _ = p.A`,
	}
	for filename, contents := range files {
		path := filepath.Join(rootPath, filename)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	if err := conn.Call(ctx, "initialize", lsp.InitializeParams{RootPath: rootPath}, nil); err != nil {
		t.Fatal("initialize:", err)
	}

	// Test some hovers using files on disk.
	wantHover := map[string]string{
		"a.go:1:17":    "func A()",
		"b.go:1:20":    "func A()",
		"p2/c.go:1:40": "func A()",
	}
	lspTests(t, ctx, conn, rootPath, wantHover, nil, nil, nil, nil, nil)

	// Now mimic what happens when a file is edited but not yet
	// saved. It should re-typecheck using the unsaved file contents.
	if err := conn.Call(ctx, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:  "file://" + filepath.Join(rootPath, "a.go"),
			Text: "package p; func A() int { return 0 }",
		},
	}, nil); err != nil {
		t.Fatal("textDocument/didOpen:", err)
	}
	wantHover = map[string]string{
		"a.go:1:17":    "func A() int",
		"b.go:1:20":    "func A() int",
		"p2/c.go:1:40": "func A() int",
	}
	lspTests(t, ctx, conn, rootPath, wantHover, nil, nil, nil, nil, nil)
}
