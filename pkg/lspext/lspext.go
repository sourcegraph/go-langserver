package lspext

import (
	"github.com/sourcegraph/go-lsp/lspext"
)

// WorkspaceSymbolParams is the extension workspace/symbol parameter type.
type WorkspaceSymbolParams = lspext.WorkspaceSymbolParams

// WorkspaceReferencesParams is parameters for the `workspace/xreferences` extension
//
// See: https://github.com/sourcegraph/language-server-protocol/blob/master/extension-workspace-reference.md
//
type WorkspaceReferencesParams = lspext.WorkspaceReferencesParams

// ReferenceInformation represents information about a reference to programming
// constructs like variables, classes, interfaces etc.
type ReferenceInformation = lspext.ReferenceInformation

// SymbolDescriptor represents information about a programming construct like a
// variable, class, interface, etc that has a reference to it. It is up to the
// language server to define the schema of this object.
//
// SymbolDescriptor usually uniquely identifies a symbol, but it is not
// guaranteed to do so.
type SymbolDescriptor = lspext.SymbolDescriptor

// SymbolLocationInformation is the response type for the `textDocument/xdefinition` extension.
type SymbolLocationInformation = lspext.SymbolLocationInformation
