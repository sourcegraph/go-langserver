package lspext

import "github.com/sourcegraph/go-langserver/pkg/lsp"

// See https://github.com/sourcegraph/language-server-protocol/pull/4.

type ContentParams struct {
	TextDocument lsp.TextDocumentIdentifier `json:"textDocument"`
}

type Content struct {
	Text string `json:"text"`
}

type GlobParams struct {
	Patterns []string `json:"patterns"`
}
