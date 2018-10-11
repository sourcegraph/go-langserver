package langserver

import (
	"testing"

	"github.com/sourcegraph/go-lsp"
)

type applyContentChangesTestCase struct {
	code     string
	changes  []lsp.TextDocumentContentChangeEvent
	expected string
}

var applyContentChangesTestCases = map[string]applyContentChangesTestCase{
	"add new line at end": applyContentChangesTestCase{
		code: "package langserver\n",
		changes: []lsp.TextDocumentContentChangeEvent{
			toContentChange(toRange(1, 0, 1, 0), 0, "\n"),
		},
		expected: "package langserver\n\n",
	},
	"remove line": applyContentChangesTestCase{
		code: "package langserver\n\n// my comment\n",
		changes: []lsp.TextDocumentContentChangeEvent{
			toContentChange(toRange(2, 0, 3, 0), 14, ""),
		},
		expected: "package langserver\n\n",
	},
	"add code in line": applyContentChangesTestCase{
		code: "package langserver\n\n// my comment\n",
		changes: []lsp.TextDocumentContentChangeEvent{
			toContentChange(toRange(2, 6, 2, 6), 0, "awesome "),
		},
		expected: "package langserver\n\n// my awesome comment\n",
	},
	"replace code in line": applyContentChangesTestCase{
		code: "package langserver\n\n// my awesome comment\n",
		changes: []lsp.TextDocumentContentChangeEvent{
			toContentChange(toRange(2, 6, 2, 13), 7, "terrible"),
		},
		expected: "package langserver\n\n// my terrible comment\n",
	},
	"complete replace and change afterwards": applyContentChangesTestCase{
		code: "package langserver\n\n// some code ...\n",
		changes: []lsp.TextDocumentContentChangeEvent{
			// complete replace of the contents
			lsp.TextDocumentContentChangeEvent{Range: nil, RangeLength: 0, Text: "package langserver_2\n"},
			// with an additional change afterwards
			toContentChange(toRange(1, 0, 1, 0), 0, "\n// code for langserver_2\n"),
		},
		expected: "package langserver_2\n\n// code for langserver_2\n",
	},
}

func TestApplyContentChanges(t *testing.T) {
	for label, test := range applyContentChangesTestCases {
		t.Run(label, func(t *testing.T) {
			newCode, err := applyContentChanges(lsp.DocumentURI("/src/langserver.go"), []byte(test.code), test.changes)
			if err != nil {
				t.Error(err)
			}
			if string(newCode) != test.expected {
				t.Errorf("Expected %q but got %q", test.expected, newCode)
			}
		})
	}
}

func toContentChange(r lsp.Range, rl uint, t string) lsp.TextDocumentContentChangeEvent {
	return lsp.TextDocumentContentChangeEvent{Range: &r, RangeLength: rl, Text: t}
}

func toRange(sl, sc, el, ec int) lsp.Range {
	return lsp.Range{Start: toPosition(sl, sc), End: toPosition(el, ec)}
}

func toPosition(l, c int) lsp.Position {
	return lsp.Position{Line: l, Character: c}
}
