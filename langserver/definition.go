package langserver

import (
	"context"
	"go/ast"
	"log"

	"github.com/sourcegraph/go-langserver/langserver/internal/refs"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *LangHandler) handleDefinition(ctx context.Context, conn JSONRPC2Conn, req *jsonrpc2.Request, params lsp.TextDocumentPositionParams) (*lsp.Location, error) {
	res, err := h.handleXDefinition(ctx, conn, req, params)
	if err != nil {
		return nil, err
	}
	return &res.Location, nil
}

func (h *LangHandler) handleXDefinition(ctx context.Context, conn JSONRPC2Conn, req *jsonrpc2.Request, params lsp.TextDocumentPositionParams) (*symbolLocationInformation, error) {
	rootPath := h.FilePath(h.init.RootPath)
	bctx := h.BuildContext(ctx)

	fset, nodeList, pathEnclosingInterval, _, pkg, err := h.typecheck(ctx, conn, params.TextDocument.URI, params.Position)
	if err != nil {
		// Invalid nodes means we tried to click on something which is
		// not an ident (eg comment/string/etc). Return no locations.
		if _, ok := err.(*invalidNodeError); ok {
			return &symbolLocationInformation{}, nil
		}
		return nil, err
	}

	obj, ok := pkg.Uses[nodeList]
	if !ok {
		obj, ok = pkg.Defs[nodeList]
	}
	p := obj.Pos()
	if ok && obj != nil {
		if !p.IsValid() {
			// Builtins have an invalid Pos. Just don't emit a definition for
			// them, for now. It's not that valuable to jump to their def.
			//
			// TODO(sqs): find a way to actually emit builtin locations
			// (pointing to builtin/builtin.go).
			return &symbolLocationInformation{}, nil
		}
	}

	node := ast.Ident{NamePos: p, Name: obj.Name()}

	findPackage := h.getFindPackageFunc()
	// Determine location information for the node.
	l := symbolLocationInformation{
		Location: goRangeToLSPLocation(fset, node.Pos(), node.End()),
	}

	// Determine metadata information for the node.
	if def, err := refs.DefInfo(pkg.Pkg, &pkg.Info, pathEnclosingInterval, node.Pos()); err == nil {
		symDesc, err := defSymbolDescriptor(ctx, bctx, rootPath, *def, findPackage)
		if err != nil {
			// TODO: tracing
			log.Println("refs.DefInfo:", err)
		} else {
			l.Symbol = symDesc
		}
	} else {
		// TODO: tracing
		log.Println("refs.DefInfo:", err)
	}

	return &l, nil
}
