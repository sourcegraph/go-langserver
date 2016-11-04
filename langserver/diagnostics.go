package langserver

import (
	"context"
	"fmt"
	"go/scanner"
	"go/token"
	"go/types"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

type diagnostics map[string][]*lsp.Diagnostic // map of URI to diagnostics (for PublishDiagnosticParams)

// publishDiagnostics sends diagnostic information (such as compile
// errors) to the client.
func (h *LangHandler) publishDiagnostics(ctx context.Context, conn JSONRPC2Conn, diags diagnostics) error {
	for filename, diags := range diags {
		params := lsp.PublishDiagnosticsParams{
			URI:         "file://" + filename,
			Diagnostics: make([]lsp.Diagnostic, len(diags)),
		}
		for i, d := range diags {
			params.Diagnostics[i] = *d
		}
		if err := conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
			return err
		}
	}
	return nil
}

func errsToDiagnostics(typeErrs []error) (diagnostics, error) {
	var diags diagnostics
	for _, typeErr := range typeErrs {
		var (
			p   token.Position
			msg string
		)
		switch e := typeErr.(type) {
		case types.Error:
			p = e.Fset.Position(e.Pos)
			msg = e.Msg
		case scanner.Error:
			p = e.Pos
			msg = e.Msg
		case scanner.ErrorList:
			if len(e) == 0 {
				continue
			}
			p = e[0].Pos
			msg = e[0].Msg
			if len(e) > 1 {
				msg = fmt.Sprintf("%s (and %d more errors)", msg, len(e)-1)
			}
		default:
			return nil, fmt.Errorf("unexpected type error: %#+v", typeErr)
		}
		if diags == nil {
			diags = diagnostics{}
		}
		diag := &lsp.Diagnostic{
			Range: lsp.Range{
				// LSP is 0-indexed, so subtract one from the numbers Go
				// reports.
				Start: lsp.Position{Line: p.Line - 1, Character: p.Column - 1},
				End:   lsp.Position{Line: p.Line - 1, Character: p.Column - 1},
			},
			Severity: lsp.Error,
			Source:   "go",
			Message:  strings.TrimSpace(msg),
		}
		diags[p.Filename] = append(diags[p.Filename], diag)
	}
	return diags, nil
}
