package langserver

import (
	"context"
	"fmt"
	"go/scanner"
	"go/token"
	"go/types"
	"strings"
	"sync"

	"golang.org/x/tools/go/loader"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/sourcegraph/go-langserver/langserver/util"
)

type diagnostics map[string][]*lsp.Diagnostic // map of URI to diagnostics (for PublishDiagnosticParams)

type diagnosticsCache struct {
	mu    sync.Mutex
	cache diagnostics
}

func newDiagnosticsCache() *diagnosticsCache {
	return &diagnosticsCache{
		cache: diagnostics{},
	}
}

// sync updates the diagnosticsCache and returns diagnostics need to be
// published.
//
// When a file no longer has any diagnostics the file will be removed from
// the cache. The removed file will be included in the returned diagnostics
// in order to clear the client.
//
// sync is thread safe and will only allow one go routine to modify the cache
// at a time.
func (p *diagnosticsCache) sync(update func(diagnostics) diagnostics, compare func(oldDiagnostics, newDiagnostics diagnostics) (publish diagnostics)) (publish diagnostics) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cache == nil {
		p.cache = diagnostics{}
	}

	newCache := update(p.cache)
	publish = compare(p.cache, newCache)
	p.cache = newCache
	return publish
}

// publishDiagnostics sends diagnostic information (such as compile
// errors) to the client.
func (h *LangHandler) publishDiagnostics(ctx context.Context, conn jsonrpc2.JSONRPC2, diags diagnostics, source string, files []string) error {
	if !h.config.DiagnosticsEnabled {
		return nil
	}

	if diags == nil {
		diags = diagnostics{}
	}

	publish := h.diagnosticsCache.sync(
		func(cached diagnostics) diagnostics {
			return updateCachedDiagnostics(cached, diags, source, files)
		},
		func(oldDiagnostics, newDiagnostics diagnostics) (publish diagnostics) {
			return compareCachedDiagnostics(oldDiagnostics, newDiagnostics, files)
		},
	)

	for filename, diags := range publish {
		params := lsp.PublishDiagnosticsParams{
			URI:         util.PathToURI(filename),
			Diagnostics: make([]lsp.Diagnostic, len(diags)),
		}
		for i, d := range diags {
			params.Diagnostics[i] = *d
		}
		if err := conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
			return err
		}
	}
	return nil
}

func updateCachedDiagnostics(cachedDiagnostics diagnostics, newDiagnostics diagnostics, source string, files []string) diagnostics {
	// copy cachedDiagnostics so we don't mutate it
	cache := make(diagnostics, len(cachedDiagnostics))
	for k, v := range cachedDiagnostics {
		cache[k] = v
	}

	for _, file := range files {

		// remove all of the diagnostics for the given source/file combinations and add the new diagnostics to the cache.
		i := 0
		for _, diag := range cache[file] {
			if diag.Source != source {
				cache[file][i] = diag
				i++
			}
		}
		cache[file] = append(cache[file][:i], newDiagnostics[file]...)

		// clear out empty cache
		if len(cache[file]) == 0 {
			delete(cache, file)
		}
	}

	return cache
}

// compareCachedDiagnostics compares new and old diagnostics to determine
// what needs to be published.
func compareCachedDiagnostics(oldDiagnostics, newDiagnostics diagnostics, files []string) (publish diagnostics) {
	publish = diagnostics{}
	for _, f := range files {
		_, inOld := oldDiagnostics[f]
		diags, inNew := newDiagnostics[f]

		if inOld && !inNew {
			publish[f] = nil
			continue
		}

		if inNew {
			publish[f] = diags
		}
	}

	return publish
}

func errsToDiagnostics(typeErrs []error, prog *loader.Program) (diagnostics, error) {
	var diags diagnostics
	for _, typeErr := range typeErrs {
		var (
			p    token.Position
			pEnd token.Position
			msg  string
		)
		switch e := typeErr.(type) {
		case types.Error:
			p = e.Fset.Position(e.Pos)
			_, path, _ := prog.PathEnclosingInterval(e.Pos, e.Pos)
			if len(path) > 0 {
				pEnd = e.Fset.Position(path[0].End())
			}
			msg = e.Msg
		case scanner.Error:
			p = e.Pos
			msg = e.Msg
		case scanner.ErrorList:
			if len(e) == 0 {
				continue
			}
			p = e[0].Pos
			msg = e[0].Msg
			if len(e) > 1 {
				msg = fmt.Sprintf("%s (and %d more errors)", msg, len(e)-1)
			}
		default:
			return nil, fmt.Errorf("unexpected type error: %#+v", typeErr)
		}
		// LSP is 0-indexed, so subtract one from the numbers Go reports.
		start := lsp.Position{Line: p.Line - 1, Character: p.Column - 1}
		end := lsp.Position{Line: pEnd.Line - 1, Character: pEnd.Column - 1}
		if !pEnd.IsValid() {
			end = start
		}
		diag := &lsp.Diagnostic{
			Range: lsp.Range{
				Start: start,
				End:   end,
			},
			Severity: lsp.Error,
			Source:   "go",
			Message:  strings.TrimSpace(msg),
		}
		if diags == nil {
			diags = diagnostics{}
		}
		diags[p.Filename] = append(diags[p.Filename], diag)
	}
	return diags, nil
}
