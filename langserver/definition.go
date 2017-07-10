package langserver

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"path"

	"github.com/sourcegraph/go-langserver/langserver/internal/godef"
	"github.com/sourcegraph/go-langserver/langserver/internal/refs"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *LangHandler) handleDefinition(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.TextDocumentPositionParams) ([]lsp.Location, error) {
	_, _, locs, err := h.definitionGodef(ctx, params)
	return locs, err
}

func (h *LangHandler) definitionGodef(ctx context.Context, params lsp.TextDocumentPositionParams) (*token.FileSet, *godef.Result, []lsp.Location, error) {
	// Read file contents and calculate byte offset.
	contents, err := h.readFile(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, nil, nil, err
	}
	filename := h.FilePath(params.TextDocument.URI)
	offset, valid, why := offsetForPosition(contents, params.Position)
	if !valid {
		return nil, nil, nil, fmt.Errorf("invalid position: %s:%d:%d (%s)", filename, params.Position.Line, params.Position.Character, why)
	}

	// Invoke godef to determine the position of the definition.
	bctx := h.BuildContext(ctx)
	fset := token.NewFileSet()
	findPackage := h.getFindPackageFunc()
	res, err := godef.Godef(ctx, bctx, fset, offset, filename, contents, h.FS, godef.FindPackageFunc(findPackage))
	if err != nil {
		return nil, nil, nil, err
	}
	if res.Package != nil {
		// TODO: return directory location. This right now at least matches our
		// other implementation.
		return fset, res, []lsp.Location{}, nil
	}
	loc := goRangeToLSPLocation(fset, res.Start, res.End)

	if loc.URI == "file://" {
		// TODO: builtins do not have valid URIs or locations, so we emit a
		// phony location here instead. This is better than our other
		// implementation.
		loc.URI = pathToURI(path.Join(bctx.GOROOT, "/src/builtin/builtin.go"))
		loc.Range = lsp.Range{}
	}

	return fset, res, []lsp.Location{loc}, nil
}

func (h *LangHandler) handleXDefinition(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.TextDocumentPositionParams) ([]symbolLocationInformation, error) {
	if !isFileURI(params.TextDocument.URI) {
		return nil, &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidParams,
			Message: fmt.Sprintf("%s not yet supported for out-of-workspace URI (%q)", req.Method, params.TextDocument.URI),
		}
	}

	rootPath := h.FilePath(h.init.RootPath)
	bctx := h.BuildContext(ctx)

	fset, node, pathEnclosingInterval, _, pkg, _, err := h.typecheck(ctx, conn, params.TextDocument.URI, params.Position)
	if err != nil {
		// Invalid nodes means we tried to click on something which is
		// not an ident (eg comment/string/etc). Return no locations.
		if _, ok := err.(*invalidNodeError); ok {
			return []symbolLocationInformation{}, nil
		}
		return nil, err
	}

	var nodes []*ast.Ident
	obj, ok := pkg.Uses[node]
	if !ok {
		obj, ok = pkg.Defs[node]
	}
	if ok && obj != nil {
		if p := obj.Pos(); p.IsValid() {
			nodes = append(nodes, &ast.Ident{NamePos: p, Name: obj.Name()})
		} else {
			// Builtins have an invalid Pos. Just don't emit a definition for
			// them, for now. It's not that valuable to jump to their def.
			//
			// TODO(sqs): find a way to actually emit builtin locations
			// (pointing to builtin/builtin.go).
			return []symbolLocationInformation{}, nil
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("definition not found")
	}
	findPackage := h.getFindPackageFunc()
	locs := make([]symbolLocationInformation, 0, len(nodes))
	for _, node := range nodes {
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
		locs = append(locs, l)
	}
	return locs, nil
}
