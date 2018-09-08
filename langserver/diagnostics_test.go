package langserver

import (
	"reflect"
	"testing"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

type updateCachedDiagnosticsTestCase struct {
	cache  diagnostics
	diags  diagnostics
	source string
	files  []string

	expectedCache diagnostics
	expectedDiags diagnostics
}

var updateCachedDiagnosticsTestCases = map[string]updateCachedDiagnosticsTestCase{
	"add to cache": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{},
		diags:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "go"}}},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "go"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "go"}}},
	},
	"add to cache multi source": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}}},
		diags:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar", Source: "go"}}},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "bar", Source: "go"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "bar", Source: "go"}}},
	},
	"update cache": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "go"}}},
		diags:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar", Source: "go"}}},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar", Source: "go"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar", Source: "go"}}},
	},
	"update cache multi source": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "will be updated", Source: "go"}}},
		diags:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "updated", Source: "go"}}},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "updated", Source: "go"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "updated", Source: "go"}}},
	},
	"remove from cache": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "go"}}},
		diags:  diagnostics{},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{},
		expectedDiags: diagnostics{"a.go": nil}, // clears the client cache
	},
	"remove from cache  multi source": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}, {Message: "bar", Source: "go"}}},
		diags:  diagnostics{},
		source: "go",
		files:  []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo", Source: "lint"}}},
	},
	"add, change and remove from cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "will be updated", Source: "go"}},
			"c.go": []*lsp.Diagnostic{{Message: "will be removed", Source: "go"}},
			// d.go no diagnostics yet
		},
		diags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated", Source: "go"}},
			// c.go no diagnostics anymore
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
		},
		source: "go",
		files:  []string{"a.go", "b.go", "c.go", "d.go"},

		expectedCache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated", Source: "go"}},
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
		},
		expectedDiags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated", Source: "go"}},
			"c.go": nil, // clears the client cache
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
		},
	},
	"add, change and remove from cache multi source": updateCachedDiagnosticsTestCase{
		cache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}, {Message: "same", Source: "lint"}},
			"b.go": []*lsp.Diagnostic{{Message: "will be updated", Source: "go"}, {Message: "same", Source: "lint"}},
			"c.go": []*lsp.Diagnostic{{Message: "will be removed", Source: "go"}, {Message: "same", Source: "lint"}},
			// d.go no diagnostics yet
			"e.go": []*lsp.Diagnostic{{Message: "will be removed", Source: "go"}},
		},
		diags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated", Source: "go"}},
			// c.go no diagnostics anymore
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
			// e.go no diagnostics anymore
		},
		source: "go",
		files:  []string{"a.go", "b.go", "c.go", "d.go", "e.go"},

		expectedCache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}, {Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}, {Message: "updated", Source: "go"}},
			"c.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}},
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
		},
		expectedDiags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}, {Message: "same", Source: "go"}},
			"b.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}, {Message: "updated", Source: "go"}},
			"c.go": []*lsp.Diagnostic{{Message: "same", Source: "lint"}},
			"d.go": []*lsp.Diagnostic{{Message: "added", Source: "go"}},
			"e.go": nil, // clears the client cache
		},
	},
	"do not set nil if not in cache": updateCachedDiagnosticsTestCase{
		cache:  diagnostics{},
		diags:  diagnostics{},
		source: "go",
		files:  []string{"a.go", "b.go"},

		expectedCache: diagnostics{},
		expectedDiags: diagnostics{}, // nothing, because a.go and b.go are not part of the cache
	},
}

func TestSyncCachedDiagnosticsTestCases(t *testing.T) {
	for label, test := range updateCachedDiagnosticsTestCases {
		t.Run(label, func(t *testing.T) {
			updatedCache := syncCachedDiagnostics(test.cache, test.diags, test.source, test.files)

			if !reflect.DeepEqual(test.expectedCache, updatedCache) {
				t.Errorf("Cached diagnostics does not match\nExpected: %v\nActual: %v", test.expectedCache, updatedCache)
			}

			if !reflect.DeepEqual(test.expectedDiags, test.diags) {
				t.Errorf("Reported diagnostics does not match\nExpected: %v\nActual: %v", test.expectedDiags, test.diags)
			}
		})
	}
}
