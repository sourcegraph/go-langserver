package lspext

import "github.com/sourcegraph/go-langserver/pkg/lsp"

// PartialResultParams is the input for "$/partialResult", a notification.
type PartialResultParams struct {
	ID      lsp.ID      `json:"id"`
	Patches []JSONPatch `json:"patches"`
}

type JSONPatch struct {
	OP    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
