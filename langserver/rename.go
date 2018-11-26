package langserver

import (
	"context"

	lsp "github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *LangHandler) handleRename(ctx context.Context, conn jsonrpc2.JSONRPC2,
	req *jsonrpc2.Request, params lsp.RenameParams) (lsp.WorkspaceEdit, error) {
	rp := lsp.ReferenceParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: params.TextDocument,
			Position:     params.Position,
		},
		Context: lsp.ReferenceContext{
			IncludeDeclaration: true,
			XLimit:             0,
		},
	}

	references, err := h.handleTextDocumentReferences(ctx, conn, req, rp)
	if err != nil {
		return lsp.WorkspaceEdit{}, err
	}

	result := lsp.WorkspaceEdit{}
	if result.Changes == nil {
		result.Changes = make(map[string][]lsp.TextEdit)
	}
	for _, ref := range references {
		edit := lsp.TextEdit{
			Range:   ref.Range,
			NewText: params.NewName,
		}
		edits := result.Changes[string(ref.URI)]
		if edits == nil {
			edits = []lsp.TextEdit{}
		}
		edits = append(edits, edit)
		result.Changes[string(ref.URI)] = edits
	}
	return result, nil
}
