package langserver

import (
	"reflect"
	"sort"
	"testing"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

func Test_resultSorter(t *testing.T) {
	type testcase struct {
		rawQuery   string
		allSymbols []lsp.SymbolInformation
		expResults []lsp.SymbolInformation
	}
	tests := []testcase{{
		rawQuery: "foo.bar",
		allSymbols: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "foo", Name: "Bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "foo",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "asdf",
			Location: lsp.Location{URI: "foo.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "one", Name: "two",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
		expResults: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "Bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "foo",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "asdf",
			Location: lsp.Location{URI: "foo.go"},
			Kind:     lsp.SKFunction,
		}},
	}, {
		rawQuery: "foo bar",
		allSymbols: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "foo",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "asdf",
			Location: lsp.Location{URI: "foo.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "one", Name: "two",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
		expResults: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "foo",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}, {
			ContainerName: "asdf", Name: "asdf",
			Location: lsp.Location{URI: "foo.go"},
			Kind:     lsp.SKFunction,
		}},
	}, {
		// Just tests that 'is:exported' does not affect resultSorter
		// results, as filtering is done elsewhere in (*LangHandler).collectFromPkg
		rawQuery: "is:exported",
		allSymbols: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
		expResults: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
	}, {
		rawQuery: "",
		allSymbols: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
		expResults: []lsp.SymbolInformation{{
			ContainerName: "foo", Name: "bar",
			Location: lsp.Location{URI: "file.go"},
			Kind:     lsp.SKFunction,
		}},
	}}

	for _, test := range tests {
		results := resultSorter{Query: ParseQuery(test.rawQuery)}
		for _, s := range test.allSymbols {
			results.Collect(s)
		}
		sort.Sort(&results)
		if !reflect.DeepEqual(results.Results(), test.expResults) {
			t.Errorf("got %+v, but wanted %+v", results.Results(), test.expResults)
		}
	}
}

func TestQueryString(t *testing.T) {
	tests := []struct {
		input, expect string
	}{
		// ---
		// test case sensitive flag...
		// check default value
		// check each override
		// check what we get back
		// check CaseSensitive: true ensures file name case is preserved
		{input: "bar baz", expect: "bar baz CaseSensitive:false"},
		{input: "CaseSensitive:true bar baz", expect: "bar baz CaseSensitive:true"},
		{input: "CaseSensitive:false bar baz", expect: "bar baz CaseSensitive:false"},
		{input: "CaseSensitive:true bar baz Baz BAz file:fileCaseSensitive", expect: "bar baz Baz BAz file:fileCaseSensitive CaseSensitive:true"},
		{input: "CaseSensitive:false bar baz Baz BAz file:fileCaseInsensitive", expect: "bar baz baz baz file:filecaseinsensitive CaseSensitive:false"},
		// ---

		// Basic queries.
		{input: "foo bar", expect: "foo bar CaseSensitive:false"},
		{input: "func bar", expect: "func bar CaseSensitive:false"},
		{input: "is:exported", expect: "is:exported CaseSensitive:false"},
		{input: "dir:foo", expect: "dir:foo CaseSensitive:false"},
		{input: "is:exported bar", expect: "is:exported bar CaseSensitive:false"},
		{input: "dir:foo bar", expect: "dir:foo bar CaseSensitive:false"},
		{input: "is:exported bar baz", expect: "is:exported bar baz CaseSensitive:false"},
		{input: "dir:foo bar baz", expect: "dir:foo bar baz CaseSensitive:false"},

		// Test guarantee of byte-wise ordering (hint: we only guarantee logical
		// equivalence, not byte-wise equality).
		{input: "bar baz is:exported", expect: "is:exported bar baz CaseSensitive:false"},
		{input: "bar baz dir:foo", expect: "dir:foo bar baz CaseSensitive:false"},
		{input: "func baz dir:foo", expect: "dir:foo func baz CaseSensitive:false"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := ParseQuery(test.input).String()
			if got != test.expect {
				t.Errorf("got %q, expect %q", got, test.expect)
			}
		})
	}
}
