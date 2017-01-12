package langserver

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"golang.org/x/tools/go/buildutil"

	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *LangHandler) handleTextDocumentFormatting(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.DocumentFormattingParams) ([]lsp.TextEdit, error) {
	if !util.IsURI(params.TextDocument.URI) {
		return nil, &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidParams,
			Message: fmt.Sprintf("%s not yet supported for out-of-workspace URI (%q)", req.Method, params.TextDocument.URI),
		}
	}

	filename := h.FilePath(params.TextDocument.URI)
	bctx := h.BuildContext(ctx)
	fset := token.NewFileSet()
	file, err := buildutil.ParseFile(fset, bctx, nil, path.Dir(filename), path.Base(filename), parser.ParseComments)
	if err != nil {
		return nil, err
	}

	ast.SortImports(fset, file)

	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	err = cfg.Fprint(&buf, fset, file)
	if err != nil {
		return nil, err
	}

	b := buf.Bytes()
	a, err := h.readFile(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(b, a) {
		return nil, nil
	}

	// LSP wants a list of TextEdits. We use difflib to compute a
	// non-naive TextEdit. Originally we returned an edit which deleted
	// everything followed by inserting everything. This leads to a poor
	// experience in vscode.
	as := strings.Split(string(a), "\n")
	bs := strings.Split(string(b), "\n")
	m := difflib.NewMatcher(as, bs)
	var edits []lsp.TextEdit
	for _, op := range m.GetOpCodes() {
		switch op.Tag {
		case 'r': // 'r' (replace):  a[i1:i2] should be replaced by b[j1:j2]
			edits = append(edits, lsp.TextEdit{
				Range: lsp.Range{
					Start: lsp.Position{
						Line: op.I1,
					},
					End: lsp.Position{
						Line: op.I2,
					},
				},
				NewText: strings.Join(bs[op.J1:op.J2], "\n") + "\n",
			})
		case 'd': // 'd' (delete):   a[i1:i2] should be deleted, j1==j2 in this case.
			edits = append(edits, lsp.TextEdit{
				Range: lsp.Range{
					Start: lsp.Position{
						Line: op.I1,
					},
					End: lsp.Position{
						Line: op.I2,
					},
				},
			})
		case 'i': // 'i' (insert):   b[j1:j2] should be inserted at a[i1:i1], i1==i2 in this case.
			edits = append(edits, lsp.TextEdit{
				Range: lsp.Range{
					Start: lsp.Position{
						Line: op.I1,
					},
					End: lsp.Position{
						Line: op.I1,
					},
				},
				NewText: strings.Join(bs[op.J1:op.J2], "\n") + "\n",
			})
		}
	}

	return edits, nil
}
