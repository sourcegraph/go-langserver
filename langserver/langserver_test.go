package langserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/sourcegraph/ctxvfs"
	"github.com/sourcegraph/go-langserver/langserver/internal/gocode"
	"github.com/sourcegraph/go-langserver/langserver/internal/utils"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/go-langserver/pkg/lspext"
	"github.com/sourcegraph/jsonrpc2"
)

type serverTestCase struct {
	skip    bool
	rootURI lsp.DocumentURI
	fs      map[string]string
	mountFS map[string]map[string]string // mount dir -> map VFS
	cases   lspTestCases
}

var serverTestCases = map[string]serverTestCase{
	"go basic": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": "package p; func A() { A() }",
			"b.go": "package p; func B() { A() }",
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				//"a.go:1:9":  "package p", // TODO(slimsag): sub-optimal "no declaration found for p"
				"a.go:1:17": "func A()",
				"a.go:1:23": "func A()",
				"b.go:1:17": "func B()",
				"b.go:1:23": "func A()",
			},
			wantHover: map[string]string{
				"a.go:1:9":  "package p",
				"a.go:1:17": "func A()",
				"a.go:1:23": "func A()",
				"b.go:1:17": "func B()",
				"b.go:1:23": "func A()",
			},
			wantDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17-1:18",
				"a.go:1:23": "/src/test/pkg/a.go:1:17-1:18",
				"b.go:1:17": "/src/test/pkg/b.go:1:17-1:18",
				"b.go:1:23": "/src/test/pkg/a.go:1:17-1:18",
			},
			wantXDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17 id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
				"a.go:1:23": "/src/test/pkg/a.go:1:17 id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
				"b.go:1:17": "/src/test/pkg/b.go:1:17 id:test/pkg/-/B name:B package:test/pkg packageName:p recv: vendor:false",
				"b.go:1:23": "/src/test/pkg/a.go:1:17 id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
			},
			wantCompletion: map[string]string{
				//"a.go:1:24": "1:23-1:24 A function func()", // returns empty list for unknown reason. Works if the two statements are in seperate lines
				"b.go:1:24": "1:23-1:24 A function func()",
			},
			wantReferences: map[string][]string{
				"a.go:1:17": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
				"a.go:1:23": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
				"b.go:1:17": []string{"/src/test/pkg/b.go:1:17"},
				"b.go:1:23": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:function:A:1:17"},
				"b.go": []string{"/src/test/pkg/b.go:function:B:1:17"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Query: "A"}:           []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Query: "B"}:           []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Query: "is:exported"}: []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Query: "dir:/"}:       []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Query: "dir:/ A"}:     []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Query: "dir:/ B"}:     []string{"/src/test/pkg/b.go:function:B:1:17"},

				// non-nil SymbolDescriptor + no keys.
				{Symbol: make(lspext.SymbolDescriptor)}: []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},

				// Individual filter fields.
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg"}}: []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"name": "A"}}:           []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"name": "B"}}:           []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"packageName": "p"}}:    []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"recv": ""}}:            []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"vendor": false}}:       []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},

				// Combined filter fields.
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg"}}:                                                               []string{"/src/test/pkg/a.go:function:A:1:17", "/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "A"}}:                                                  []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "A", "packageName": "p"}}:                              []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "A", "packageName": "p", "recv": ""}}:                  []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "A", "packageName": "p", "recv": "", "vendor": false}}: []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "B"}}:                                                  []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "B", "packageName": "p"}}:                              []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "B", "packageName": "p", "recv": ""}}:                  []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "B", "packageName": "p", "recv": "", "vendor": false}}: []string{"/src/test/pkg/b.go:function:B:1:17"},

				// By ID.
				{Symbol: lspext.SymbolDescriptor{"id": "test/pkg/-/B"}}: []string{"/src/test/pkg/b.go:function:B:1:17"},
				{Symbol: lspext.SymbolDescriptor{"id": "test/pkg/-/A"}}: []string{"/src/test/pkg/a.go:function:A:1:17"},
			},
			wantFormatting: map[string]string{
				"a.go": "package p\n\nfunc A() { A() }\n",
			},
		},
	},
	"go detailed": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": "package p; type T struct { F string }",
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				// "a.go:1:28": "(T).F string", // TODO(sqs): see golang/hover.go; this is the output we want
				"a.go:1:28": "struct field F string",
				"a.go:1:17": `type T struct; struct{ F string }`,
			},

			wantHover: map[string]string{
				// "a.go:1:28": "(T).F string", // TODO(sqs): see golang/hover.go; this is the output we want
				"a.go:1:28": "struct field F string",
				"a.go:1:17": `type T struct; struct {
    F string
}`,
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:class:T:1:17"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/a.go:class:T:1:17"},
				{Query: "T"}:           []string{"/src/test/pkg/a.go:class:T:1:17"},
				{Query: "F"}:           []string{}, // we don't return fields for now
				{Query: "is:exported"}: []string{"/src/test/pkg/a.go:class:T:1:17"},
			},
		},
	},
	"exported defs unexported type": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": "package p; type t struct { F string }",
		},
		cases: lspTestCases{
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:class:t:1:17"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: "is:exported"}: []string{},
			},
		},
	},
	"go xtest": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go":      "package p; var A int",
			"x_test.go": `package p_test; import "test/pkg"; var X = p.A`,
			"y_test.go": "package p_test; func Y() int { return X }",

			// non xtest to ensure we don't mix up xtest and test.
			"a_test.go": `package p; var X = A`,
			"b_test.go": "package p; func Y() int { return X }",
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"a.go:1:16":      "var A int",
				"x_test.go:1:40": "var X = p.A",
				"x_test.go:1:46": "var A int",
				"a_test.go:1:16": "var X = A",
				"a_test.go:1:20": "var A int",
			},

			wantHover: map[string]string{
				"a.go:1:16":      "var A int",
				"x_test.go:1:40": "var X int",
				"x_test.go:1:46": "var A int",
				"a_test.go:1:16": "var X int",
				"a_test.go:1:20": "var A int",
			},
			wantCompletion: map[string]string{
				"x_test.go:1:45": "1:44-1:45 panic function func(interface{}), print function func(...interface{}), println function func(...interface{}), p module ",
				"x_test.go:1:46": "1:46-1:46 A variable int",
				"b_test.go:1:35": "1:34-1:35 X variable int",
			},
			wantSymbols: map[string][]string{
				"y_test.go": []string{"/src/test/pkg/y_test.go:function:Y:1:22"},
				"b_test.go": []string{"/src/test/pkg/b_test.go:function:Y:1:17"},
			},
			wantReferences: map[string][]string{
				"a.go:1:16": []string{
					"/src/test/pkg/a.go:1:16",
					"/src/test/pkg/a_test.go:1:20",
					"/src/test/pkg/x_test.go:1:46",
				},
				"x_test.go:1:46": []string{
					"/src/test/pkg/a.go:1:16",
					"/src/test/pkg/a_test.go:1:20",
					"/src/test/pkg/x_test.go:1:46",
				},
				"x_test.go:1:40": []string{
					"/src/test/pkg/x_test.go:1:40",
					"/src/test/pkg/y_test.go:1:39",
				},

				// The same as the xtest references above, but in the normal test pkg.
				"a_test.go:1:20": []string{
					"/src/test/pkg/a.go:1:16",
					"/src/test/pkg/a_test.go:1:20",
					"/src/test/pkg/x_test.go:1:46",
				},
				"a_test.go:1:16": []string{
					"/src/test/pkg/a_test.go:1:16",
					"/src/test/pkg/b_test.go:1:34",
				},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/x_test.go:1:24-1:34 -> id:test/pkg name: package:test/pkg packageName:p recv: vendor:false",
					"/src/test/pkg/x_test.go:1:46-1:47 -> id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
				},
			},
		},
	},
	"go test": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go":      "package p; var A int",
			"a_test.go": `package p; import "test/pkg/b"; var X = b.B; func TestB() {}`,
			"b/b.go":    "package b; var B int; func C() int { return B };",
			"c/c.go":    `package c; import "test/pkg/b"; var X = b.B;`,
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"a_test.go:1:37": "var X = b.B",
				"a_test.go:1:43": "var B int",
			},
			wantHover: map[string]string{
				"a_test.go:1:37": "var X int",
				"a_test.go:1:43": "var B int",
			},
			wantReferences: map[string][]string{
				"a_test.go:1:43": []string{
					"/src/test/pkg/a_test.go:1:43",
					"/src/test/pkg/b/b.go:1:16",
					"/src/test/pkg/b/b.go:1:45",
					"/src/test/pkg/c/c.go:1:43",
				},
				"a_test.go:1:41": []string{
					"/src/test/pkg/a_test.go:1:19",
					"/src/test/pkg/a_test.go:1:41",
				},
				"a_test.go:1:51": []string{
					"/src/test/pkg/a_test.go:1:51",
				},
			},
		},
	},
	"go subdirectory in repo": {
		rootURI: "file:///src/test/pkg/d",
		fs: map[string]string{
			"a.go":    "package d; func A() { A() }",
			"d2/b.go": `package d2; import "test/pkg/d"; func B() { d.A(); B() }`,
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a.go:1:17":    "func A()",
				"a.go:1:23":    "func A()",
				"d2/b.go:1:39": "func B()",
				"d2/b.go:1:47": "func A()",
				"d2/b.go:1:52": "func B()",
			},
			wantDefinition: map[string]string{
				"a.go:1:17":    "/src/test/pkg/d/a.go:1:17-1:18",
				"a.go:1:23":    "/src/test/pkg/d/a.go:1:17-1:18",
				"d2/b.go:1:39": "/src/test/pkg/d/d2/b.go:1:39-1:40",
				"d2/b.go:1:47": "/src/test/pkg/d/a.go:1:17-1:18",
				"d2/b.go:1:52": "/src/test/pkg/d/d2/b.go:1:39-1:40",
			},
			wantXDefinition: map[string]string{
				"a.go:1:17":    "/src/test/pkg/d/a.go:1:17 id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				"a.go:1:23":    "/src/test/pkg/d/a.go:1:17 id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				"d2/b.go:1:39": "/src/test/pkg/d/d2/b.go:1:39 id:test/pkg/d/d2/-/B name:B package:test/pkg/d/d2 packageName:d2 recv: vendor:false",
				"d2/b.go:1:47": "/src/test/pkg/d/a.go:1:17 id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				"d2/b.go:1:52": "/src/test/pkg/d/d2/b.go:1:39 id:test/pkg/d/d2/-/B name:B package:test/pkg/d/d2 packageName:d2 recv: vendor:false",
			},
			wantCompletion: map[string]string{
				"d2/b.go:1:47": "1:47-1:47 A function func()",
				//"d2/b.go:1:52": "1:52-1:52 d module , B function func()", // B not presented, see test case "go simple"
			},
			wantSymbols: map[string][]string{
				"a.go":    []string{"/src/test/pkg/d/a.go:function:A:1:17"},
				"d2/b.go": []string{"/src/test/pkg/d/d2/b.go:function:B:1:39"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/d/a.go:function:A:1:17", "/src/test/pkg/d/d2/b.go:function:B:1:39"},
				{Query: "is:exported"}: []string{"/src/test/pkg/d/a.go:function:A:1:17", "/src/test/pkg/d/d2/b.go:function:B:1:39"},
				{Query: "dir:"}:        []string{"/src/test/pkg/d/a.go:function:A:1:17"},
				{Query: "dir:/"}:       []string{"/src/test/pkg/d/a.go:function:A:1:17"},
				{Query: "dir:."}:       []string{"/src/test/pkg/d/a.go:function:A:1:17"},
				{Query: "dir:./"}:      []string{"/src/test/pkg/d/a.go:function:A:1:17"},
				{Query: "dir:/d2"}:     []string{"/src/test/pkg/d/d2/b.go:function:B:1:39"},
				{Query: "dir:./d2"}:    []string{"/src/test/pkg/d/d2/b.go:function:B:1:39"},
				{Query: "dir:d2/"}:     []string{"/src/test/pkg/d/d2/b.go:function:B:1:39"},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				// Non-matching name query.
				{Query: lspext.SymbolDescriptor{"name": "nope"}}: []string{},

				// Matching against invalid field name.
				{Query: lspext.SymbolDescriptor{"nope": "A"}}: []string{},

				// Matching against an invalid dirs hint.
				{Query: lspext.SymbolDescriptor{"package": "test/pkg/d"}, Hints: map[string]interface{}{"dirs": []string{"file:///src/test/pkg/d/d3"}}}: []string{},

				// Matching against a dirs hint with multiple dirs.
				{Query: lspext.SymbolDescriptor{"package": "test/pkg/d"}, Hints: map[string]interface{}{"dirs": []string{"file:///src/test/pkg/d/d2", "file:///src/test/pkg/d/invalid"}}}: []string{
					"/src/test/pkg/d/d2/b.go:1:20-1:32 -> id:test/pkg/d name: package:test/pkg/d packageName:d recv: vendor:false",
					"/src/test/pkg/d/d2/b.go:1:47-1:48 -> id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				},

				// Matching against a dirs hint.
				{Query: lspext.SymbolDescriptor{"package": "test/pkg/d"}, Hints: map[string]interface{}{"dirs": []string{"file:///src/test/pkg/d/d2"}}}: []string{
					"/src/test/pkg/d/d2/b.go:1:20-1:32 -> id:test/pkg/d name: package:test/pkg/d packageName:d recv: vendor:false",
					"/src/test/pkg/d/d2/b.go:1:47-1:48 -> id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				},

				// Matching against single field.
				{Query: lspext.SymbolDescriptor{"package": "test/pkg/d"}}: []string{
					"/src/test/pkg/d/d2/b.go:1:20-1:32 -> id:test/pkg/d name: package:test/pkg/d packageName:d recv: vendor:false",
					"/src/test/pkg/d/d2/b.go:1:47-1:48 -> id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				},

				// Matching against no fields.
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/d/d2/b.go:1:20-1:32 -> id:test/pkg/d name: package:test/pkg/d packageName:d recv: vendor:false",
					"/src/test/pkg/d/d2/b.go:1:47-1:48 -> id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false",
				},
				{
					Query: lspext.SymbolDescriptor{
						"name":        "",
						"package":     "test/pkg/d",
						"packageName": "d",
						"recv":        "",
						"vendor":      false,
					},
				}: []string{"/src/test/pkg/d/d2/b.go:1:20-1:32 -> id:test/pkg/d name: package:test/pkg/d packageName:d recv: vendor:false"},
				{
					Query: lspext.SymbolDescriptor{
						"name":        "A",
						"package":     "test/pkg/d",
						"packageName": "d",
						"recv":        "",
						"vendor":      false,
					},
				}: []string{"/src/test/pkg/d/d2/b.go:1:47-1:48 -> id:test/pkg/d/-/A name:A package:test/pkg/d packageName:d recv: vendor:false"},
			},
		},
	},
	"go multiple packages in dir": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": "package p; func A() { A() }",
			"main.go": `// +build ignore

package main; import "test/pkg"; func B() { p.A(); B() }`,
		},
		cases: lspTestCases{

			wantHover: map[string]string{
				"a.go:1:17": "func A()",
				"a.go:1:23": "func A()",
				// Not parsing build-tag-ignored files:
				//
				// "main.go:3:39": "func B()", // func B()
				// "main.go:3:47": "func A()", // p.A()
				// "main.go:3:52": "func B()", // B()
			},
			wantDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17-1:18",
				"a.go:1:23": "/src/test/pkg/a.go:1:17-1:18",
				// Not parsing build-tag-ignored files:
				//
				// "main.go:3:39": "/src/test/pkg/main.go:3:39", // B() -> func B()
				// "main.go:3:47": "/src/test/pkg/a.go:1:17",    // p.A() -> a.go func A()
				// "main.go:3:52": "/src/test/pkg/main.go:3:39", // B() -> func B()
			},
			wantXDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17 id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
				"a.go:1:23": "/src/test/pkg/a.go:1:17 id:test/pkg/-/A name:A package:test/pkg packageName:p recv: vendor:false",
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:function:A:1:17"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Query: "is:exported"}: []string{"/src/test/pkg/a.go:function:A:1:17"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "A", "packageName": "p", "recv": "", "vendor": false}}: []string{"/src/test/pkg/a.go:function:A:1:17"},
			},
		},
	},
	"goroot": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package p; import "fmt"; var _ = fmt.Println; var x int`,
		},
		mountFS: map[string]map[string]string{
			"/goroot": {
				"src/fmt/print.go":       "package fmt; func Println(a ...interface{}) (n int, err error) { return }",
				"src/builtin/builtin.go": "package builtin; type int int",
			},
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"a.go:1:40": "func Println(a ...interface{}) (n int, err error); Println formats using the default formats for its operands and writes to standard output. Spaces are always added between operands and a newline is appended. It returns the number of bytes written and any write error encountered. \n\n",
				// "a.go:1:53": "type int int",
			},
			wantHover: map[string]string{
				"a.go:1:40": "func Println(a ...interface{}) (n int, err error)",
				// "a.go:1:53": "type int int",
			},
			overrideGodefDefinition: map[string]string{
				"a.go:1:40": "/goroot/src/fmt/print.go:256:6-256:13",  // hitting the real GOROOT
				"a.go:1:53": "/goroot/src/builtin/builtin.go:1:1-1:1", // TODO: accurate builtin positions
			},
			wantDefinition: map[string]string{
				"a.go:1:40": "/goroot/src/fmt/print.go:1:19-1:26",
				// "a.go:1:53": "/goroot/src/builtin/builtin.go:TODO:TODO", // TODO(sqs): support builtins
			},
			wantXDefinition: map[string]string{
				"a.go:1:40": "/goroot/src/fmt/print.go:1:19 id:fmt/-/Println name:Println package:fmt packageName:fmt recv: vendor:false",
			},
			wantCompletion: map[string]string{
				// use default GOROOT, since gocode needs package binaries
				"a.go:1:21": "1:20-1:21 flag module , fmt module ",
				"a.go:1:44": "1:38-1:44 Println function func(a ...interface{}) (n int, err error)",
			},
			wantSymbols: map[string][]string{
				"a.go": []string{
					"/src/test/pkg/a.go:variable:_:1:26",
					"/src/test/pkg/a.go:variable:x:1:47",
				},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}: []string{
					"/src/test/pkg/a.go:variable:_:1:26",
					"/src/test/pkg/a.go:variable:x:1:47",
				},
				{Query: "is:exported"}: []string{},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "x", "packageName": "p", "recv": "", "vendor": false}}: []string{"/src/test/pkg/a.go:variable:x:1:47"},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:24 -> id:fmt name: package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/a.go:1:38-1:45 -> id:fmt/-/Println name:Println package:fmt packageName:fmt recv: vendor:false",
				},
			},
		},
	},
	"gopath": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a/a.go": `package a; func A() {}`,
			"b/b.go": `package b; import "test/pkg/a"; var _ = a.A`,
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a/a.go:1:17": "func A()",
				// "b/b.go:1:20": "package", // TODO(sqs): make import paths hoverable
				"b/b.go:1:43": "func A()",
			},
			wantDefinition: map[string]string{
				"a/a.go:1:17": "/src/test/pkg/a/a.go:1:17-1:18",
				// "b/b.go:1:20": "/src/test/pkg/a", // TODO(sqs): make import paths hoverable
				"b/b.go:1:43": "/src/test/pkg/a/a.go:1:17-1:18",
			},
			wantXDefinition: map[string]string{
				"a/a.go:1:17": "/src/test/pkg/a/a.go:1:17 id:test/pkg/a/-/A name:A package:test/pkg/a packageName:a recv: vendor:false",
				"b/b.go:1:43": "/src/test/pkg/a/a.go:1:17 id:test/pkg/a/-/A name:A package:test/pkg/a packageName:a recv: vendor:false",
			},
			wantCompletion: map[string]string{
				"b/b.go:1:26": "1:20-1:26 test/pkg/a module , test/pkg/b module ",
				"b/b.go:1:43": "1:43-1:43 A function func()",
			},
			wantReferences: map[string][]string{
				"a/a.go:1:17": []string{
					"/src/test/pkg/a/a.go:1:17",
					"/src/test/pkg/b/b.go:1:43",
				},
				"b/b.go:1:43": []string{ // calling "references" on call site should return same result as on decl
					"/src/test/pkg/a/a.go:1:17",
					"/src/test/pkg/b/b.go:1:43",
				},
				"b/b.go:1:41": []string{ // calling "references" on package
					"/src/test/pkg/b/b.go:1:19",
					"/src/test/pkg/b/b.go:1:41",
				},
			},
			wantSymbols: map[string][]string{
				"a/a.go": []string{"/src/test/pkg/a/a.go:function:A:1:17"},
				"b/b.go": []string{"/src/test/pkg/b/b.go:variable:_:1:33"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/a/a.go:function:A:1:17", "/src/test/pkg/b/b.go:variable:_:1:33"},
				{Query: "is:exported"}: []string{"/src/test/pkg/a/a.go:function:A:1:17"},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/b/b.go:1:19-1:31 -> id:test/pkg/a name: package:test/pkg/a packageName:a recv: vendor:false",
					"/src/test/pkg/b/b.go:1:43-1:44 -> id:test/pkg/a/-/A name:A package:test/pkg/a packageName:a recv: vendor:false",
				},
			},
		},
	},
	"go vendored dep": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package a; import "github.com/v/vendored"; var _ = vendored.V`,
			"vendor/github.com/v/vendored/v.go": "package vendored; func V() {}",
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a.go:1:61": "func V()",
			},
			wantDefinition: map[string]string{
				"a.go:1:61": "/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24-1:25",
				//"a.go:1:40": "/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24-1:25",
			},
			wantXDefinition: map[string]string{
				"a.go:1:61": "/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24 id:test/pkg/vendor/github.com/v/vendored/-/V name:V package:test/pkg/vendor/github.com/v/vendored packageName:vendored recv: vendor:true",
			},
			wantCompletion: map[string]string{
				"a.go:1:34": "1:20-1:34 github.com/v/vendored module ",
				"a.go:1:61": "1:61-1:61 V function func()",
			},
			wantReferences: map[string][]string{
				"vendor/github.com/v/vendored/v.go:1:24": []string{
					"/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24",
					"/src/test/pkg/a.go:1:61",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:variable:_:1:44"},
				"vendor/github.com/v/vendored/v.go": []string{"/src/test/pkg/vendor/github.com/v/vendored/v.go:function:V:1:24"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/a.go:variable:_:1:44", "/src/test/pkg/vendor/github.com/v/vendored/v.go:function:V:1:24"},
				{Query: "is:exported"}: []string{},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg", "name": "_", "packageName": "a", "recv": "", "vendor": false}}:                                     []string{"/src/test/pkg/a.go:variable:_:1:44"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg/vendor/github.com/v/vendored", "name": "V", "packageName": "vendored", "recv": "", "vendor": true}}:  []string{"/src/test/pkg/vendor/github.com/v/vendored/v.go:function:V:1:24"},
				{Symbol: lspext.SymbolDescriptor{"package": "test/pkg/vendor/github.com/v/vendored", "name": "V", "packageName": "vendored", "recv": "", "vendor": false}}: []string{},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:42 -> id:test/pkg/vendor/github.com/v/vendored name: package:test/pkg/vendor/github.com/v/vendored packageName:vendored recv: vendor:true",
					"/src/test/pkg/a.go:1:61-1:62 -> id:test/pkg/vendor/github.com/v/vendored/-/V name:V package:test/pkg/vendor/github.com/v/vendored packageName:vendored recv: vendor:true",
				},
			},
		},
	},
	"go vendor symbols with same name": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"z.go": `package pkg; func x() bool { return true }`,
			"vendor/github.com/a/pkg2/x.go": `package pkg2; func x() bool { return true }`,
			"vendor/github.com/x/pkg3/x.go": `package pkg3; func x() bool { return true }`,
		},
		cases: lspTestCases{
			wantSymbols: map[string][]string{
				"z.go": []string{"/src/test/pkg/z.go:function:x:1:19"},
				"vendor/github.com/a/pkg2/x.go": []string{"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:x:1:20"},
				"vendor/github.com/x/pkg3/x.go": []string{"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:x:1:20"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}: []string{
					"/src/test/pkg/z.go:function:x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:x:1:20",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:x:1:20",
				},
				{Query: "x"}: []string{
					"/src/test/pkg/z.go:function:x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:x:1:20",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:x:1:20",
				},
				{Query: "pkg2.x"}: []string{
					"/src/test/pkg/z.go:function:x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:x:1:20",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:x:1:20",
				},
				{Query: "pkg3.x"}: []string{
					"/src/test/pkg/z.go:function:x:1:19",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:x:1:20",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:x:1:20",
				},
				{Query: "is:exported"}: []string{},
			},
		},
	},
	"go external dep": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package a; import "github.com/d/dep"; var _ = dep.D; var _ = dep.D`,
		},
		mountFS: map[string]map[string]string{
			"/src/github.com/d/dep": {
				"d.go": "package dep; func D() {}; var _ = D",
			},
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a.go:1:51": "func D()",
			},
			wantDefinition: map[string]string{
				"a.go:1:51": "/src/github.com/d/dep/d.go:1:19-1:20",
			},
			wantXDefinition: map[string]string{
				"a.go:1:51": "/src/github.com/d/dep/d.go:1:19 id:github.com/d/dep/-/D name:D package:github.com/d/dep packageName:dep recv: vendor:false",
			},
			wantCompletion: map[string]string{
				"a.go:1:34": "1:20-1:34 github.com/d/dep module ",
				"a.go:1:51": "1:51-1:51 D function func()",
			},
			wantReferences: map[string][]string{
				"a.go:1:51": []string{
					"/src/test/pkg/a.go:1:51",
					"/src/test/pkg/a.go:1:66",
					// Do not include "refs" from the dependency
					// package itself; only return results in the
					// workspace.
				},
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:37 -> id:github.com/d/dep name: package:github.com/d/dep packageName:dep recv: vendor:false",
					"/src/test/pkg/a.go:1:51-1:52 -> id:github.com/d/dep/-/D name:D package:github.com/d/dep packageName:dep recv: vendor:false",
					"/src/test/pkg/a.go:1:66-1:67 -> id:github.com/d/dep/-/D name:D package:github.com/d/dep packageName:dep recv: vendor:false",
				},
			},
		},
	},
	"external dep with vendor": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package p; import "github.com/d/dep"; var _ = dep.D().F`,
		},
		mountFS: map[string]map[string]string{
			"/src/github.com/d/dep": map[string]string{
				"d.go":               `package dep; import "vendp"; func D() (v vendp.V) { return }`,
				"vendor/vendp/vp.go": "package vendp; type V struct { F int }",
			},
		},
		cases: lspTestCases{
			wantDefinition: map[string]string{
				"a.go:1:55": "/src/github.com/d/dep/vendor/vendp/vp.go:1:32-1:33",
			},
			wantXDefinition: map[string]string{
				"a.go:1:55": "/src/github.com/d/dep/vendor/vendp/vp.go:1:32 id:github.com/d/dep/vendor/vendp/-/V/F name:F package:github.com/d/dep/vendor/vendp packageName:vendp recv:V vendor:true",
			},
			wantCompletion: map[string]string{
				"a.go:1:55": "1:55-1:55 F variable int",
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:37 -> id:github.com/d/dep name: package:github.com/d/dep packageName:dep recv: vendor:false",
					"/src/test/pkg/a.go:1:55-1:56 -> id:github.com/d/dep/vendor/vendp/-/V/F name:F package:github.com/d/dep/vendor/vendp packageName:vendp recv:V vendor:true",
					"/src/test/pkg/a.go:1:51-1:52 -> id:github.com/d/dep/-/D name:D package:github.com/d/dep packageName:dep recv: vendor:false",
				},
			},
		},
	},
	"go external dep at subtree": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package a; import "github.com/d/dep/subp"; var _ = subp.D`,
		},
		mountFS: map[string]map[string]string{
			"/src/github.com/d/dep": {
				"subp/d.go": "package subp; func D() {}",
			},
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a.go:1:57": "func D()",
			},
			wantDefinition: map[string]string{
				"a.go:1:57": "/src/github.com/d/dep/subp/d.go:1:20-1:21",
			},
			wantXDefinition: map[string]string{
				"a.go:1:57": "/src/github.com/d/dep/subp/d.go:1:20 id:github.com/d/dep/subp/-/D name:D package:github.com/d/dep/subp packageName:subp recv: vendor:false",
			},
			wantCompletion: map[string]string{
				"a.go:1:34": "1:20-1:34 github.com/d/dep/subp module ",
				"a.go:1:57": "1:57-1:57 D function func()",
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:42 -> id:github.com/d/dep/subp name: package:github.com/d/dep/subp packageName:subp recv: vendor:false",
					"/src/test/pkg/a.go:1:57-1:58 -> id:github.com/d/dep/subp/-/D name:D package:github.com/d/dep/subp packageName:subp recv: vendor:false",
				},
			},
		},
	},
	"go nested external dep": { // a depends on dep1, dep1 depends on dep2
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package a; import "github.com/d/dep1"; var _ = dep1.D1().D2`,
		},
		mountFS: map[string]map[string]string{
			"/src/github.com/d/dep1": {
				"d1.go": `package dep1; import "github.com/d/dep2"; func D1() dep2.D2 { return dep2.D2{} }`,
			},
			"/src/github.com/d/dep2": {
				"d2.go": "package dep2; type D2 struct { D2 int }",
			},
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"a.go:1:53": "func D1() dep2.D2",
				"a.go:1:59": "struct field D2 int",
			},
			wantHover: map[string]string{
				"a.go:1:53": "func D1() D2",
				"a.go:1:59": "struct field D2 int",
			},
			wantDefinition: map[string]string{
				"a.go:1:53": "/src/github.com/d/dep1/d1.go:1:48-1:50", // func D1
				"a.go:1:58": "/src/github.com/d/dep2/d2.go:1:32-1:34", // field D2
			},
			wantXDefinition: map[string]string{
				"a.go:1:53": "/src/github.com/d/dep1/d1.go:1:48 id:github.com/d/dep1/-/D1 name:D1 package:github.com/d/dep1 packageName:dep1 recv: vendor:false",
				"a.go:1:58": "/src/github.com/d/dep2/d2.go:1:32 id:github.com/d/dep2/-/D2/D2 name:D2 package:github.com/d/dep2 packageName:dep2 recv:D2 vendor:false",
			},
			wantCompletion: map[string]string{
				//"a.go:1:53": "1:53-1:53 D1 function func() D2", // gocode does not handle D2 correctly
				"a.go:1:58": "1:58-1:58 D2 variable int",
			},
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:38 -> id:github.com/d/dep1 name: package:github.com/d/dep1 packageName:dep1 recv: vendor:false",
					"/src/test/pkg/a.go:1:58-1:60 -> id:github.com/d/dep2/-/D2/D2 name:D2 package:github.com/d/dep2 packageName:dep2 recv:D2 vendor:false",
					"/src/test/pkg/a.go:1:53-1:55 -> id:github.com/d/dep1/-/D1 name:D1 package:github.com/d/dep1 packageName:dep1 recv: vendor:false",
				},
			},
		},
	},
	"go symbols": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"abc.go": `package a

type XYZ struct {}

func (x XYZ) ABC() {}
`,
			"bcd.go": `package a

type YZA struct {}

func (y YZA) BCD() {}
`,
			"xyz.go": `package a

func yza() {}
`,
		},
		cases: lspTestCases{
			wantSymbols: map[string][]string{
				"abc.go": []string{"/src/test/pkg/abc.go:class:XYZ:3:6", "/src/test/pkg/abc.go:method:XYZ.ABC:5:14"},
				"bcd.go": []string{"/src/test/pkg/bcd.go:class:YZA:3:6", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14"},
				"xyz.go": []string{"/src/test/pkg/xyz.go:function:yza:3:6"},
			},
			wantWorkspaceSymbols: map[*lspext.WorkspaceSymbolParams][]string{
				{Query: ""}:            []string{"/src/test/pkg/abc.go:class:XYZ:3:6", "/src/test/pkg/bcd.go:class:YZA:3:6", "/src/test/pkg/xyz.go:function:yza:3:6", "/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14"},
				{Query: "xyz"}:         []string{"/src/test/pkg/abc.go:class:XYZ:3:6", "/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/xyz.go:function:yza:3:6"},
				{Query: "yza"}:         []string{"/src/test/pkg/bcd.go:class:YZA:3:6", "/src/test/pkg/xyz.go:function:yza:3:6", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14"},
				{Query: "abc"}:         []string{"/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/abc.go:class:XYZ:3:6"},
				{Query: "bcd"}:         []string{"/src/test/pkg/bcd.go:method:YZA.BCD:5:14", "/src/test/pkg/bcd.go:class:YZA:3:6"},
				{Query: "is:exported"}: []string{"/src/test/pkg/abc.go:class:XYZ:3:6", "/src/test/pkg/bcd.go:class:YZA:3:6", "/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14"},
			},
		},
	},
	"go hover docs": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `// Copyright 2015 someone.
// Copyrights often span multiple lines.

// Some additional non-package docs.

// Package p is a package with lots of great things.
package p

import "github.com/a/pkg2"

// logit is pkg2.X
var logit = pkg2.X

// T is a struct.
type T struct {
	// F is a string field.
	F string

	// H is a header.
	H pkg2.Header
}

// Foo is the best string.
var Foo string

var (
	// I1 is an int
	I1 = 1

	// I2 is an int
	I2 = 3
)
`,
			"vendor/github.com/a/pkg2/x.go": `// Package pkg2 shows dependencies.
//
// How to
//
// 	Example Code!
//
package pkg2

// A comment that should be ignored

// X does the unknown.
func X() {
	panic("zomg")
}

// Header is like an HTTP header, only better.
type Header struct {
	// F is a string, too.
	F string
}
`,
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				//"a.go:7:9": "package p; Package p is a package with lots of great things. \n\n", // TODO(slimsag): sub-optimal "no declaration found for p"
				//"a.go:9:9": "", TODO: handle hovering on import statements (ast.BasicLit)
				"a.go:12:5":  "var logit = pkg2.X; logit is pkg2.X \n\n",
				"a.go:12:13": "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:12:18": "func X(); X does the unknown. \n\n",
				"a.go:15:6":  "type T struct; T is a struct. \n\n; struct {\n\t// F is a string field.\n\tF string\n\n\t// H is a header.\n\tH pkg2.Header\n}",
				"a.go:17:2":  "struct field F string; F is a string field. \n\n",
				"a.go:20:2":  "struct field H pkg2.Header; H is a header. \n\n",
				"a.go:20:4":  "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:24:5":  "var Foo string; Foo is the best string. \n\n",
				"a.go:31:2":  "var I2 = 3; I2 is an int \n\n",
			},
			wantHover: map[string]string{
				"a.go:7:9": "package p; Package p is a package with lots of great things. \n\n",
				//"a.go:9:9": "", TODO: handle hovering on import statements (ast.BasicLit)
				"a.go:12:5":  "var logit func(); logit is pkg2.X \n\n",
				"a.go:12:13": "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:12:18": "func X(); X does the unknown. \n\n",
				"a.go:15:6":  "type T struct; T is a struct. \n\n; struct {\n    F string\n    H Header\n}",
				"a.go:17:2":  "struct field F string; F is a string field. \n\n",
				"a.go:20:2":  "struct field H test/pkg/vendor/github.com/a/pkg2.Header; H is a header. \n\n",
				"a.go:20:4":  "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:24:5":  "var Foo string; Foo is the best string. \n\n",
				"a.go:31:2":  "var I2 int; I2 is an int \n\n",
			},
		},
	},
	"workspace references multiple files": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package p; import "fmt"; var _ = fmt.Println; var x int`,
			"b.go": `package p; import "fmt"; var _ = fmt.Println; var y int`,
			"c.go": `package p; import "fmt"; var _ = fmt.Println; var z int`,
		},
		mountFS: map[string]map[string]string{
			"/goroot": {
				"src/fmt/print.go":       "package fmt; func Println(a ...interface{}) (n int, err error) { return }",
				"src/builtin/builtin.go": "package builtin; type int int",
			},
		},
		cases: lspTestCases{
			wantWorkspaceReferences: map[*lspext.WorkspaceReferencesParams][]string{
				{Query: lspext.SymbolDescriptor{}}: []string{
					"/src/test/pkg/a.go:1:19-1:24 -> id:fmt name: package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/a.go:1:38-1:45 -> id:fmt/-/Println name:Println package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/b.go:1:19-1:24 -> id:fmt name: package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/b.go:1:38-1:45 -> id:fmt/-/Println name:Println package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/c.go:1:19-1:24 -> id:fmt name: package:fmt packageName:fmt recv: vendor:false",
					"/src/test/pkg/c.go:1:38-1:45 -> id:fmt/-/Println name:Println package:fmt packageName:fmt recv: vendor:false",
				},
			},
		},
	},
	"signatures": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package p

				// Comments for A
				func A(foo int, bar func(baz int) int) int {
					return bar(foo)
				}


				func B() {}

				// Comments for C
				func C(x int, y int) int {
					return x+y
				}`,
			"b.go": "package p; func main() { B(); A(); A(0,); A(0); C(1,2) }",
		},
		cases: lspTestCases{
			wantSignatures: map[string]string{
				"b.go:1:28": "func() 0",
				"b.go:1:33": "func(foo int, bar func(baz int) int) int Comments for A\n 0",
				"b.go:1:40": "func(foo int, bar func(baz int) int) int Comments for A\n 1",
				"b.go:1:46": "func(foo int, bar func(baz int) int) int Comments for A\n 0",
				"b.go:1:51": "func(x int, y int) int Comments for C\n 0",
				"b.go:1:53": "func(x int, y int) int Comments for C\n 1",
				"b.go:1:54": "func(x int, y int) int Comments for C\n 1",
			},
		},
	},
	"completion": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": `package p

import "strings"

func s2() {
	_ = strings.Title("s")
	_ = new(strings.Replacer)
}

const s1 = 42

var s3 int
var s4 func()`,
		},
		cases: lspTestCases{
			wantCompletion: map[string]string{
				"a.go:6:7":   "6:6-6:7 s1 constant , s2 function func(), strings module , string class built-in, s3 variable int, s4 variable func()",
				"a.go:7:7":   "7:6-7:7 nil constant , new function func(type) *type",
				"a.go:12:11": "12:8-12:11 int class built-in, int16 class built-in, int32 class built-in, int64 class built-in, int8 class built-in",
			},
		},
	},
	"unexpected paths": {
		// notice the : and @ symbol
		rootURI: "file:///src/t:est/@hello/pkg",
		skip:    runtime.GOOS == "windows", // this test is not supported on windows
		fs: map[string]string{
			"a.go": "package p; func A() { A() }",
		},
		cases: lspTestCases{
			wantHover: map[string]string{
				"a.go:1:17": "func A()",
			},
			wantReferences: map[string][]string{
				"a.go:1:17": []string{
					"/src/t:est/@hello/pkg/a.go:1:17",
					"/src/t:est/@hello/pkg/a.go:1:23",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/t:est/@hello/pkg/a.go:function:A:1:17"},
			},
		},
	},
	"hover fail issue 223": {
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"main.go": `package main

import (
	"fmt"
)

func main() {

	b := &Hello{
		a: 1,
	}

	fmt.Println(b.Bye())
}

type Hello struct {
	a int
}

func (h *Hello) Bye() int {
	return h.a
}
`,
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"main.go:13:17": "func (h *Hello) Bye() int",
			},
			wantHover: map[string]string{
				"main.go:13:17": "func (*Hello).Bye() int",
			},
		},
	},
}

func TestServer(t *testing.T) {
	for label, test := range serverTestCases {
		t.Run(label, func(t *testing.T) {
			if test.skip {
				t.Skip()
				return
			}

			cfg := NewDefaultConfig()
			cfg.FuncSnippetEnabled = true
			cfg.GocodeCompletionEnabled = true

			h := &LangHandler{
				config:        cfg,
				HandlerShared: &HandlerShared{},
			}

			addr, done := startServer(t, jsonrpc2.HandlerWithError(h.handle))
			defer done()
			conn := dialServer(t, addr)
			defer func() {
				if err := conn.Close(); err != nil {
					t.Fatal("conn.Close:", err)
				}
			}()

			rootFSPath := utils.UriToPath(test.rootURI)

			// Prepare the connection.
			ctx := context.Background()
			tdCap := lsp.TextDocumentClientCapabilities{}
			tdCap.Completion.CompletionItemKind.ValueSet = []lsp.CompletionItemKind{lsp.CIKConstant}
			if err := conn.Call(ctx, "initialize", InitializeParams{
				InitializeParams: lsp.InitializeParams{
					RootURI:      test.rootURI,
					Capabilities: lsp.ClientCapabilities{TextDocument: tdCap},
				},
				NoOSFileSystemAccess: true,
				RootImportPath:       strings.TrimPrefix(rootFSPath, "/src/"),
				BuildContext: &InitializeBuildContextParams{
					GOOS:     "linux",
					GOARCH:   "amd64",
					GOPATH:   "/",
					GOROOT:   "/goroot",
					Compiler: runtime.Compiler,
				},
			}, nil); err != nil {
				t.Fatal("initialize:", err)
			}

			h.Mu.Lock()
			h.FS.Bind(rootFSPath, mapFS(test.fs), "/", ctxvfs.BindReplace)
			for mountDir, fs := range test.mountFS {
				h.FS.Bind(mountDir, mapFS(fs), "/", ctxvfs.BindAfter)
			}
			h.Mu.Unlock()

			lspTests(t, ctx, h, conn, test.rootURI, test.cases)
		})
	}
}

func startServer(t testing.TB, h jsonrpc2.Handler) (addr string, done func()) {
	bindAddr := ":0"
	if os.Getenv("CI") != "" || runtime.GOOS == "windows" {
		// CircleCI has issues with IPv6 (e.g., "dial tcp [::]:39984:
		// connect: network is unreachable").
		// Similar error is happens on Windows:
		// "dial tcp [::]:61898: connectex: The requested address is not valid in its context."
		bindAddr = "127.0.0.1:0"
	}
	l, err := net.Listen("tcp", bindAddr)
	if err != nil {
		t.Fatal("Listen:", err)
	}
	go func() {
		if err := serve(context.Background(), l, h); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			t.Fatal("jsonrpc2.Serve:", err)
		}
	}()
	return l.Addr().String(), func() {
		if err := l.Close(); err != nil {
			t.Fatal("close listener:", err)
		}
	}
}

func serve(ctx context.Context, lis net.Listener, h jsonrpc2.Handler, opt ...jsonrpc2.ConnOpt) error {
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}
		jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), h, opt...)
	}
}

func dialServer(t testing.TB, addr string) *jsonrpc2.Conn {
	conn, err := (&net.Dialer{}).Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (interface{}, error) {
		// no-op
		return nil, nil
	}))
}

type lspTestCases struct {
	wantHover, overrideGodefHover           map[string]string
	wantDefinition, overrideGodefDefinition map[string]string
	wantXDefinition                         map[string]string
	wantCompletion                          map[string]string
	wantReferences                          map[string][]string
	wantSymbols                             map[string][]string
	wantWorkspaceSymbols                    map[*lspext.WorkspaceSymbolParams][]string
	wantSignatures                          map[string]string
	wantWorkspaceReferences                 map[*lspext.WorkspaceReferencesParams][]string
	wantFormatting                          map[string]string
}

func copyFileToOS(ctx context.Context, fs *AtomicFS, targetFile, srcFile string) error {
	src, err := fs.Open(ctx, srcFile)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func copyDirToOS(ctx context.Context, fs *AtomicFS, targetDir, srcDir string) error {
	if err := os.Mkdir(targetDir, 0777); err != nil && !os.IsExist(err) {
		return err
	}
	files, err := fs.ReadDir(ctx, srcDir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		targetPath := filepath.Join(targetDir, fi.Name())
		srcPath := path.Join(srcDir, fi.Name())
		if fi.IsDir() {
			err := copyDirToOS(ctx, fs, targetPath, srcPath)
			if err != nil {
				return err
			}
			continue
		}

		err := copyFileToOS(ctx, fs, targetPath, srcPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// lspTests runs all test suites for LSP functionality.
func lspTests(t testing.TB, ctx context.Context, h *LangHandler, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, cases lspTestCases) {
	for pos, want := range cases.wantHover {
		tbRun(t, fmt.Sprintf("hover-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			hoverTest(t, ctx, c, rootURI, pos, want)
		})
	}

	// Godef-based definition & hover testing
	wantGodefDefinition := cases.overrideGodefDefinition
	if len(wantGodefDefinition) == 0 {
		wantGodefDefinition = cases.wantDefinition
	}
	wantGodefHover := cases.overrideGodefHover
	if len(wantGodefHover) == 0 {
		wantGodefHover = cases.wantHover
	}

	if len(wantGodefDefinition) > 0 || (len(wantGodefHover) > 0 && h != nil) || len(cases.wantCompletion) > 0 {
		h.config.UseBinaryPkgCache = true

		// Copy the VFS into a temp directory, which will be our $GOPATH.
		tmpDir, err := ioutil.TempDir("", "godef-definition")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)
		if err := copyDirToOS(ctx, h.FS, tmpDir, "/"); err != nil {
			t.Fatal(err)
		}

		// Important: update build.Default.GOPATH, since it is compiled into
		// the binary we must update it here at runtime. Otherwise, godef would
		// look for $GOPATH/pkg .a files inside the $GOPATH that was set during
		// 'go test' instead of our tmp directory.
		build.Default.GOPATH = tmpDir
		gocode.SetBuildContext(&build.Default)
		tmpRootPath := filepath.Join(tmpDir, utils.UriToPath(rootURI))

		// Install all Go packages in the $GOPATH.
		oldGOPATH := os.Getenv("GOPATH")
		os.Setenv("GOPATH", tmpDir)
		out, err := exec.Command("go", "install", "-v", "...").CombinedOutput()
		os.Setenv("GOPATH", oldGOPATH)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("$ go install -v ...\n%s", out)

		testOSToVFSPath = func(osPath string) string {
			return strings.TrimPrefix(osPath, utils.UriToPath(utils.PathToURI(tmpDir)))
		}

		// Run the tests.
		for pos, want := range wantGodefDefinition {
			if strings.HasPrefix(want, "/goroot") {
				want = strings.Replace(want, "/goroot", path.Clean(utils.UriToPath(utils.PathToURI(build.Default.GOROOT))), 1)
			}
			tbRun(t, fmt.Sprintf("godef-definition-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
				definitionTest(t, ctx, c, utils.PathToURI(tmpRootPath), pos, want, tmpDir)
			})
		}
		for pos, want := range wantGodefHover {
			tbRun(t, fmt.Sprintf("godef-hover-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
				hoverTest(t, ctx, c, utils.PathToURI(tmpRootPath), pos, want)
			})
		}
		for pos, want := range cases.wantCompletion {
			tbRun(t, fmt.Sprintf("completion-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
				completionTest(t, ctx, c, utils.PathToURI(tmpRootPath), pos, want)
			})
		}

		h.config.UseBinaryPkgCache = false
	}

	for pos, want := range cases.wantDefinition {
		tbRun(t, fmt.Sprintf("definition-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			definitionTest(t, ctx, c, rootURI, pos, want, "")
		})
	}
	for pos, want := range cases.wantDefinition {
		tbRun(t, fmt.Sprintf("definition-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			definitionTest(t, ctx, c, rootURI, pos, want, "")
		})
	}
	for pos, want := range cases.wantXDefinition {
		tbRun(t, fmt.Sprintf("xdefinition-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			xdefinitionTest(t, ctx, c, rootURI, pos, want)
		})
	}

	for pos, want := range cases.wantReferences {
		tbRun(t, fmt.Sprintf("references-%s", pos), func(t testing.TB) {
			referencesTest(t, ctx, c, rootURI, pos, want)
		})
	}

	for file, want := range cases.wantSymbols {
		tbRun(t, fmt.Sprintf("symbols-%s", file), func(t testing.TB) {
			symbolsTest(t, ctx, c, rootURI, file, want)
		})
	}

	for params, want := range cases.wantWorkspaceSymbols {
		tbRun(t, fmt.Sprintf("workspaceSymbols(%v)", *params), func(t testing.TB) {
			workspaceSymbolsTest(t, ctx, c, rootURI, *params, want)
		})
	}

	for pos, want := range cases.wantSignatures {
		tbRun(t, fmt.Sprintf("signature-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			signatureTest(t, ctx, c, rootURI, pos, want)
		})
	}

	for params, want := range cases.wantWorkspaceReferences {
		tbRun(t, fmt.Sprintf("workspaceReferences"), func(t testing.TB) {
			workspaceReferencesTest(t, ctx, c, rootURI, *params, want)
		})
	}

	for file, want := range cases.wantFormatting {
		tbRun(t, fmt.Sprintf("formatting-%s", file), func(t testing.TB) {
			formattingTest(t, ctx, c, rootURI, file, want)
		})
	}
}

// tbRun calls (testing.T).Run or (testing.B).Run.
func tbRun(t testing.TB, name string, f func(testing.TB)) bool {
	switch tb := t.(type) {
	case *testing.B:
		return tb.Run(name, func(b *testing.B) { f(b) })
	case *testing.T:
		return tb.Run(name, func(t *testing.T) { f(t) })
	default:
		panic(fmt.Sprintf("unexpected %T, want *testing.B or *testing.T", tb))
	}
}

func uriJoin(base lsp.DocumentURI, file string) lsp.DocumentURI {
	return lsp.DocumentURI(string(base) + "/" + file)
}

func hoverTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	hover, err := callHover(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	if hover != want {
		t.Fatalf("got %q, want %q", hover, want)
	}
}

func definitionTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos, want, trimPrefix string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := callDefinition(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	if definition != "" {
		definition = utils.UriToPath(lsp.DocumentURI(definition))
		if trimPrefix != "" {
			definition = strings.TrimPrefix(definition, utils.UriToPath(utils.PathToURI(trimPrefix)))
		}
	}
	if definition != want {
		t.Errorf("got %q, want %q", definition, want)
	}
}

func xdefinitionTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	xdefinition, err := callXDefinition(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	xdefinition = utils.UriToPath(lsp.DocumentURI(xdefinition))
	if xdefinition != want {
		t.Errorf("\ngot  %q\nwant %q", xdefinition, want)
	}
}

func completionTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := callCompletion(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	if completion != want {
		t.Fatalf("got %q, want %q", completion, want)
	}
}

func referencesTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos string, want []string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	references, err := callReferences(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	for i := range references {
		references[i] = utils.UriToPath(lsp.DocumentURI(references[i]))
	}
	sort.Strings(references)
	sort.Strings(want)
	if !reflect.DeepEqual(references, want) {
		t.Errorf("got %q, want %q", references, want)
	}
}

func symbolsTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, file string, want []string) {
	symbols, err := callSymbols(ctx, c, uriJoin(rootURI, file))
	if err != nil {
		t.Fatal(err)
	}
	for i := range symbols {
		symbols[i] = utils.UriToPath(lsp.DocumentURI(symbols[i]))
	}
	if !reflect.DeepEqual(symbols, want) {
		t.Errorf("got %q, want %q", symbols, want)
	}
}

func workspaceSymbolsTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, params lspext.WorkspaceSymbolParams, want []string) {
	symbols, err := callWorkspaceSymbols(ctx, c, params)
	if err != nil {
		t.Fatal(err)
	}
	for i := range symbols {
		symbols[i] = utils.UriToPath(lsp.DocumentURI(symbols[i]))
	}
	if !reflect.DeepEqual(symbols, want) {
		t.Errorf("got %#v, want %q", symbols, want)
	}
}

func signatureTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := callSignature(ctx, c, uriJoin(rootURI, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	if signature != want {
		t.Fatalf("got %q, want %q", signature, want)
	}
}

func workspaceReferencesTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, params lspext.WorkspaceReferencesParams, want []string) {
	references, err := callWorkspaceReferences(ctx, c, params)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(references, want) {
		t.Errorf("\ngot  %q\nwant %q", references, want)
	}
}

func formattingTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootURI lsp.DocumentURI, file string, want string) {
	edits, err := callFormatting(ctx, c, uriJoin(rootURI, file))
	if err != nil {
		t.Fatal(err)
	}
	var got string
	switch len(edits) {
	case 0:
		// already gofmt clean
		got = ""
	case 2:
		// our implementation is dumb, it is always delete everything
		// followed by insert. Since we don't have access to the
		// input, we cheat and just look at the 2nd operation.
		got = edits[1].NewText
	default:
		t.Errorf("got %d edits, want 0 or 2", len(edits))
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func parsePos(s string) (file string, line, char int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		err = fmt.Errorf("invalid pos %q (%d parts)", s, len(parts))
		return
	}
	file = parts[0]
	line, err = strconv.Atoi(parts[1])
	if err != nil {
		err = fmt.Errorf("invalid line in %q: %s", s, err)
		return
	}
	char, err = strconv.Atoi(parts[2])
	if err != nil {
		err = fmt.Errorf("invalid char in %q: %s", s, err)
		return
	}
	return file, line - 1, char - 1, nil // LSP is 0-indexed
}

func callHover(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) (string, error) {
	var res struct {
		Contents markedStrings `json:"contents"`
		lsp.Hover
	}
	err := c.Call(ctx, "textDocument/hover", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, ms := range res.Contents {
		if i != 0 {
			str += "; "
		}
		str += ms.Value
	}
	return str, nil
}

func callDefinition(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) (string, error) {
	var res locations
	err := c.Call(ctx, "textDocument/definition", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, loc := range res {
		if loc.URI == "" {
			continue
		}
		if i != 0 {
			str += ", "
		}
		str += fmt.Sprintf("%s:%d:%d-%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1, loc.Range.End.Line+1, loc.Range.End.Character+1)
	}
	return str, nil
}

func callXDefinition(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) (string, error) {
	var res []lspext.SymbolLocationInformation
	err := c.Call(ctx, "textDocument/xdefinition", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, loc := range res {
		if loc.Location.URI == "" {
			continue
		}
		if i != 0 {
			str += ", "
		}
		str += fmt.Sprintf("%s:%d:%d %s", loc.Location.URI, loc.Location.Range.Start.Line+1, loc.Location.Range.Start.Character+1, loc.Symbol)
	}
	return str, nil
}

func callCompletion(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) (string, error) {
	var res lsp.CompletionList
	err := c.Call(ctx, "textDocument/completion", lsp.CompletionParams{TextDocumentPositionParams: lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, it := range res.Items {
		if i != 0 {
			str += ", "
		} else {
			e := it.TextEdit.Range
			str += fmt.Sprintf("%d:%d-%d:%d ", e.Start.Line+1, e.Start.Character+1, e.End.Line+1, e.End.Character+1)
		}
		str += fmt.Sprintf("%s %s %s", it.Label, it.Kind, it.Detail)
	}
	return str, nil
}

func callReferences(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) ([]string, error) {
	var res locations
	err := c.Call(ctx, "textDocument/references", lsp.ReferenceParams{
		Context: lsp.ReferenceContext{IncludeDeclaration: true},
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: line, Character: char},
		},
	}, &res)
	if err != nil {
		return nil, err
	}
	str := make([]string, len(res))
	for i, loc := range res {
		str[i] = fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}
	return str, nil
}

func callSymbols(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI) ([]string, error) {
	var symbols []lsp.SymbolInformation
	err := c.Call(ctx, "textDocument/documentSymbol", lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	}, &symbols)
	if err != nil {
		return nil, err
	}
	syms := make([]string, len(symbols))
	for i, s := range symbols {
		syms[i] = fmt.Sprintf("%s:%s:%s:%d:%d", s.Location.URI, s.Kind, qualifiedName(s), s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return syms, nil
}

func callWorkspaceSymbols(ctx context.Context, c *jsonrpc2.Conn, params lspext.WorkspaceSymbolParams) ([]string, error) {
	var symbols []lsp.SymbolInformation
	err := c.Call(ctx, "workspace/symbol", params, &symbols)
	if err != nil {
		return nil, err
	}
	syms := make([]string, len(symbols))
	for i, s := range symbols {
		syms[i] = fmt.Sprintf("%s:%s:%s:%d:%d", s.Location.URI, s.Kind, qualifiedName(s), s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return syms, nil
}

func qualifiedName(s lsp.SymbolInformation) string {
	if s.ContainerName != "" {
		return s.ContainerName + "." + s.Name
	}
	return s.Name
}

func callWorkspaceReferences(ctx context.Context, c *jsonrpc2.Conn, params lspext.WorkspaceReferencesParams) ([]string, error) {
	var references []lspext.ReferenceInformation
	err := c.Call(ctx, "workspace/xreferences", params, &references)
	if err != nil {
		return nil, err
	}
	refs := make([]string, len(references))
	for i, r := range references {
		locationURI := utils.UriToPath(r.Reference.URI)
		start := r.Reference.Range.Start
		end := r.Reference.Range.End
		refs[i] = fmt.Sprintf("%s:%d:%d-%d:%d -> %v", locationURI, start.Line+1, start.Character+1, end.Line+1, end.Character+1, r.Symbol)
	}
	return refs, nil
}

func callSignature(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI, line, char int) (string, error) {
	var res lsp.SignatureHelp
	err := c.Call(ctx, "textDocument/signatureHelp", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, si := range res.Signatures {
		if i != 0 {
			str += "; "
		}
		str += si.Label
		if si.Documentation != "" {
			str += " " + si.Documentation
		}
	}
	str += fmt.Sprintf(" %d", res.ActiveParameter)
	return str, nil
}

func callFormatting(ctx context.Context, c *jsonrpc2.Conn, uri lsp.DocumentURI) ([]lsp.TextEdit, error) {
	var edits []lsp.TextEdit
	err := c.Call(ctx, "textDocument/formatting", lsp.DocumentFormattingParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	}, &edits)
	return edits, err
}

type markedStrings []lsp.MarkedString

func (v *markedStrings) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '[' {
		var ms []markedString
		if err := json.Unmarshal(data, &ms); err != nil {
			return err
		}
		for _, ms := range ms {
			*v = append(*v, lsp.MarkedString(ms))
		}
		return nil
	}
	*v = []lsp.MarkedString{{}}
	return json.Unmarshal(data, &(*v)[0])
}

type markedString lsp.MarkedString

func (v *markedString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '{' {
		return json.Unmarshal(data, (*lsp.MarkedString)(v))
	}

	// String
	*v = markedString{}
	return json.Unmarshal(data, &v.Value)
}

type locations []lsp.Location

func (v *locations) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '[' {
		return json.Unmarshal(data, (*[]lsp.Location)(v))
	}
	*v = []lsp.Location{{}}
	return json.Unmarshal(data, &(*v)[0])
}

// testRequest is a simplified version of jsonrpc2.Request for easier
// test expectation definition and checking of the fields that matter.
type testRequest struct {
	Method string
	Params interface{}
}

func (r testRequest) String() string {
	b, err := json.Marshal(r.Params)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s(%s)", r.Method, b)
}

func testRequestEqual(a, b testRequest) bool {
	if a.Method != b.Method {
		return false
	}

	// We want to see if a and b have identical canonical JSON
	// representations. They are NOT identical Go structures, since
	// one comes from the wire (as raw JSON) and one is an interface{}
	// of a concrete struct/slice type provided as a test expectation.
	ajson, err := json.Marshal(a.Params)
	if err != nil {
		panic(err)
	}
	bjson, err := json.Marshal(b.Params)
	if err != nil {
		panic(err)
	}
	var a2, b2 interface{}
	if err := json.Unmarshal(ajson, &a2); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(bjson, &b2); err != nil {
		panic(err)
	}
	return reflect.DeepEqual(a2, b2)
}

func testRequestsEqual(as, bs []testRequest) bool {
	if len(as) != len(bs) {
		return false
	}
	for i, a := range as {
		if !testRequestEqual(a, bs[i]) {
			return false
		}
	}
	return true
}

type testRequests []testRequest

func (v testRequests) Len() int      { return len(v) }
func (v testRequests) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v testRequests) Less(i, j int) bool {
	ii, err := json.Marshal(v[i])
	if err != nil {
		panic(err)
	}
	jj, err := json.Marshal(v[j])
	if err != nil {
		panic(err)
	}
	return string(ii) < string(jj)
}

// mapFS lets us easily instantiate a VFS with a map[string]string
// (which is less noisy than map[string][]byte in test fixtures).
func mapFS(m map[string]string) ctxvfs.FileSystem {
	m2 := make(map[string][]byte, len(m))
	for k, v := range m {
		m2[k] = []byte(v)
	}
	return ctxvfs.Map(m2)
}
