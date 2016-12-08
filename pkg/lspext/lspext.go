package lspext

import "github.com/sourcegraph/go-langserver/pkg/lsp"

// WorkspaceReferencesParams is parameters for the `workspace/xreferences` extension
//
// See: https://github.com/sourcegraph/language-server-protocol/blob/master/extension-workspace-reference.md
//
type WorkspaceReferencesParams struct {
}

// ReferenceInformation is the array response type for the `workspace/xreferences` extension
//
// See: https://github.com/sourcegraph/language-server-protocol/blob/master/extension-workspace-reference.md
//
type ReferenceInformation struct {
	Reference lsp.Location     `json:"reference"`
	Symbol    SymbolDescriptor `json:"symbol"`
}

type SymbolDescriptor struct {
	Name          string                 `json:"name,omitempty"`
	Kind          lsp.SymbolKind         `json:"kind,omitempty"`
	File          string                 `json:"file,omitempty"`
	ContainerName string                 `json:"containerName,omitempty"`
	Vendor        bool                   `json:"vendor,omitempty"`
	Meta          map[string]interface{} `json:"meta,omitempty"`
}

// LocationInformation is the response type for the `textDocument/xdefinition` extension.
type LocationInformation struct {
	// A concrete location at which the definition is located, if any.
	Location lsp.Location `json:"location,omitempty"`
	// Metadata about the definition.
	Symbol SymbolDescriptor `json:"SymbolDescriptor"`
}
