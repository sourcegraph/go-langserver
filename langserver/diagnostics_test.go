package langserver

import (
	"reflect"
	"testing"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

type updateCachedDiagnosticsTestCase struct {
	cache diagnostics
	diags diagnostics
	files []string

	expectedCache diagnostics
	expectedDiags diagnostics
}

var updateCachedDiagnosticsTestCases = map[string]updateCachedDiagnosticsTestCase{
	"add to cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{},
		diags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo"}}},
		files: []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo"}}},
	},
	"update cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo"}}},
		diags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar"}}},
		files: []string{"a.go"},

		expectedCache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar"}}},
		expectedDiags: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "bar"}}},
	},
	"remove from cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{"a.go": []*lsp.Diagnostic{{Message: "foo"}}},
		diags: diagnostics{},
		files: []string{"a.go"},

		expectedCache: diagnostics{},
		expectedDiags: diagnostics{"a.go": nil}, // clears the client cache
	},
	"add, change and remove from cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same"}},
			"b.go": []*lsp.Diagnostic{{Message: "will be updated"}},
			"c.go": []*lsp.Diagnostic{{Message: "will be removed"}},
			// d.go no diagnostics yet
		},
		diags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated"}},
			// c.go no diagnostics anymore
			"d.go": []*lsp.Diagnostic{{Message: "added"}},
		},
		files: []string{"a.go", "c.go", "d.go"},

		expectedCache: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated"}},
			"d.go": []*lsp.Diagnostic{{Message: "added"}},
		},
		expectedDiags: diagnostics{
			"a.go": []*lsp.Diagnostic{{Message: "same"}},
			"b.go": []*lsp.Diagnostic{{Message: "updated"}},
			"c.go": nil, // clears the client cache
			"d.go": []*lsp.Diagnostic{{Message: "added"}},
		},
	},
	"do not set nil if not in cache": updateCachedDiagnosticsTestCase{
		cache: diagnostics{},
		diags: diagnostics{},
		files: []string{"a.go", "b.go"},

		expectedCache: diagnostics{},
		expectedDiags: diagnostics{}, // nothing, because a.go and b.go are not part of the cache
	},
}

func TestUpdateCachedDiagnosticsTestCases(t *testing.T) {
	for label, test := range updateCachedDiagnosticsTestCases {
		t.Run(label, func(t *testing.T) {
			updatedCache := updateCachedDiagnostics(test.cache, test.diags, test.files)

			if !reflect.DeepEqual(test.expectedCache, updatedCache) {
				t.Errorf("Cached diagnostics does not match\nExpected: %v\nActual: %v", test.expectedCache, updatedCache)
			}

			if !reflect.DeepEqual(test.expectedDiags, test.diags) {
				t.Errorf("Reported diagnostics does not match\nExpected: %v\nActual: %v", test.expectedDiags, test.diags)
			}
		})
	}
}
