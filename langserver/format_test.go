package langserver

import (
	"reflect"
	"testing"

	"github.com/sourcegraph/go-lsp"
)

type computeTextEditsTestCase struct {
	unformatted string
	formatted   string
	expected    []lsp.TextEdit
}

var computeTextEditsTestCases = map[string]computeTextEditsTestCase{
	"one edit": computeTextEditsTestCase{
		unformatted: "package p\n\n  func A() {}\n",
		formatted:   "package p\n\nfunc A() {}\n",
		expected: []lsp.TextEdit{
			toTextEdit(toRange(2, 0, 3, 0), "func A() {}\n"),
		},
	},
	"multiple edits": computeTextEditsTestCase{
		unformatted: "package p\n\n  func A() {}\n\n  func B() {}\n",
		formatted:   "package p\n\nfunc A() {}\n\nfunc B() {}\n",
		expected: []lsp.TextEdit{
			toTextEdit(toRange(2, 0, 3, 0), "func A() {}\n"),
			toTextEdit(toRange(4, 0, 5, 0), "func B() {}\n"),
		},
	},
	"whole text": computeTextEditsTestCase{
		unformatted: "package p; func A() {}",
		formatted:   "package langserver\n\nfunc A() {}",
		expected: []lsp.TextEdit{
			toTextEdit(toRange(0, 0, 1, 0), "package langserver\n\nfunc A() {}\n"), // TODO: why a new line?
		},
	},
}

func TestComputeEdits(t *testing.T) {
	for label, test := range computeTextEditsTestCases {
		t.Run(label, func(t *testing.T) {
			edits := ComputeTextEdits(test.unformatted, test.formatted)
			if !reflect.DeepEqual(edits, test.expected) {
				t.Errorf("Expected %q but got %q", test.expected, edits)
			}
		})
	}
}

func toTextEdit(r lsp.Range, t string) lsp.TextEdit {
	return lsp.TextEdit{Range: r, NewText: t}
}
