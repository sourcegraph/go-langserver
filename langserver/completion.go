package langserver

import (
	"context"
	"fmt"

	"github.com/sourcegraph/go-langserver/langserver/internal/gocode"
	"github.com/sourcegraph/go-langserver/langserver/internal/utils"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

var (
	GocodeCompletionEnabled = false
	CIKConstantSupported    = lsp.CIKVariable // or lsp.CIKConstant if client supported
)

func (h *LangHandler) handleTextDocumentCompletion(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.CompletionParams) (*lsp.CompletionList, error) {
	if !utils.IsURI(params.TextDocument.URI) {
		return nil, &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidParams,
			Message: fmt.Sprintf("textDocument/completion not yet supported for out-of-workspace URI (%q)", params.TextDocument.URI),
		}
	}

	// In the case of testing, our OS paths and VFS paths do not match. In the
	// real world, this is never the case. Give the test suite the opportunity
	// to correct the path now.
	vfsURI := params.TextDocument.URI
	if testOSToVFSPath != nil {
		vfsURI = utils.PathToURI(testOSToVFSPath(utils.UriToPath(vfsURI)))
	}

	// Read file contents and calculate byte offset.
	contents, err := h.readFile(ctx, vfsURI)
	if err != nil {
		return nil, err
	}
	filename := h.FilePath(params.TextDocument.URI)
	offset, valid, why := offsetForPosition(contents, params.Position)
	if !valid {
		return nil, fmt.Errorf("invalid position: %s:%d:%d (%s)", filename, params.Position.Line, params.Position.Character, why)
	}

	ca, rangelen := gocode.AutoComplete(contents, filename, offset)
	citems := make([]lsp.CompletionItem, len(ca))
	for i, it := range ca {
		var kind lsp.CompletionItemKind
		switch it.Class.String() {
		case "const":
			kind = CIKConstantSupported
		case "func":
			kind = lsp.CIKFunction
		case "import":
			kind = lsp.CIKModule
		case "package":
			kind = lsp.CIKModule
		case "type":
			kind = lsp.CIKClass
		case "var":
			kind = lsp.CIKVariable
		}
		citems[i] = lsp.CompletionItem{
			Label:  it.Name,
			Kind:   kind,
			Detail: it.Type,
			TextEdit: &lsp.TextEdit{
				Range: lsp.Range{
					Start: lsp.Position{Line: params.Position.Line, Character: params.Position.Character - rangelen},
					End:   lsp.Position{Line: params.Position.Line, Character: params.Position.Character},
				},
				NewText: it.Name,
			},
		}
	}
	return &lsp.CompletionList{
		IsIncomplete: false,
		Items:        citems,
	}, nil
}
