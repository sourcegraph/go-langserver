package gocode

import (
	"go/importer"
	"go/types"

	"github.com/sourcegraph/go-langserver/langserver/internal/gocode/gbimporter"
	"github.com/sourcegraph/go-langserver/langserver/internal/gocode/suggest"
)

type AutoCompleteRequest struct {
	Filename string
	Data     []byte
	Cursor   int
	Context  gbimporter.PackedContext
	Source   bool
	Builtin  bool
}

type AutoCompleteReply struct {
	Candidates []suggest.Candidate
	Len        int
}

func AutoComplete(req *AutoCompleteRequest) (*AutoCompleteReply, error) {
	res := &AutoCompleteReply{}
	var underlying types.ImporterFrom
	if req.Source {
		underlying = importer.For("source", nil).(types.ImporterFrom)
	} else {
		underlying = importer.Default().(types.ImporterFrom)
	}
	cfg := suggest.Config{
		Importer: gbimporter.New(&req.Context, req.Filename, underlying),
		Builtin:  req.Builtin,
	}

	candidates, d, err := cfg.Suggest(req.Filename, req.Data, req.Cursor)
	if err != nil {
		return nil, err
	}
	res.Candidates, res.Len = candidates, d
	return res, nil
}
