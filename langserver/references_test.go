package langserver

import (
	"reflect"
	"testing"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

func TestSortBySharedDirWithURI(t *testing.T) {
	got := []lsp.Location{
		lsp.Location{URI: "file:///a.go"},
		lsp.Location{URI: "file:///a/a.go"},
		lsp.Location{URI: "file:///a/a/a.go"},
		lsp.Location{URI: "file:///a/a/z.go"},
		lsp.Location{URI: "file:///a/z.go"},
		lsp.Location{URI: "file:///a/z/a.go"},
		lsp.Location{URI: "file:///a/z/z.go"},
		lsp.Location{URI: "file:///z.go"},
		lsp.Location{URI: "file:///z/a.go"},
		lsp.Location{URI: "file:///z/a/a.go"},
		lsp.Location{URI: "file:///z/a/z.go"},
		lsp.Location{URI: "file:///z/z.go"},
		lsp.Location{URI: "file:///z/z/a.go"},
		lsp.Location{URI: "file:///z/z/z.go"},
	}
	uri := "file:///z/m.go"
	want := []lsp.Location{
		lsp.Location{URI: "file:///z/a.go"},
		lsp.Location{URI: "file:///z/z.go"},
		lsp.Location{URI: "file:///z/a/a.go"},
		lsp.Location{URI: "file:///z/a/z.go"},
		lsp.Location{URI: "file:///z/z/a.go"},
		lsp.Location{URI: "file:///z/z/z.go"},
		lsp.Location{URI: "file:///a.go"},
		lsp.Location{URI: "file:///z.go"},
		lsp.Location{URI: "file:///a/a.go"},
		lsp.Location{URI: "file:///a/z.go"},
		lsp.Location{URI: "file:///a/a/a.go"},
		lsp.Location{URI: "file:///a/a/z.go"},
		lsp.Location{URI: "file:///a/z/a.go"},
		lsp.Location{URI: "file:///a/z/z.go"},
	}
	sortBySharedDirWithURI(uri, got)
	if !reflect.DeepEqual(got, want) {
		var gotURI, wantURI []string
		for _, l := range got {
			gotURI = append(gotURI, l.URI)
		}
		for _, l := range want {
			wantURI = append(wantURI, l.URI)
		}
		t.Fatalf("got != want.\nwant: %v\ngot:  %v", wantURI, gotURI)
	}
}
