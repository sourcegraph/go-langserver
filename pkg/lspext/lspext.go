package lspext

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

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

// String returns a consistently ordered string representation of the
// SymbolDescriptor. It is useful for testing.
func (s SymbolDescriptor) String() string {
	sm := make(sortedMap, 0, len(s.Meta))
	for k, v := range s.Meta {
		sm = append(sm, mapValue{key: "meta_" + k, value: v})
	}
	stdfield := func(k, v string) {
		if v != "" {
			sm = append(sm, mapValue{key: k, value: v})
		}
	}
	stdfield("name", s.Name)
	stdfield("kind", s.Kind.String())
	stdfield("file", s.File)
	stdfield("containerName", s.ContainerName)
	if s.Vendor {
		stdfield("vendor", "true")
	}
	sort.Sort(sm)
	var str string
	for _, v := range sm {
		str += fmt.Sprintf("%s:%v ", v.key, v.value)
	}
	return strings.TrimSpace(str)
}

type mapValue struct {
	key   string
	value interface{}
}

type sortedMap []mapValue

func (s sortedMap) Len() int           { return len(s) }
func (s sortedMap) Less(i, j int) bool { return s[i].key < s[j].key }
func (s sortedMap) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
