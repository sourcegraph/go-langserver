package langserver

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/refactor/importgraph"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

// documentReferencesTimeout is the timeout used for textDocument/references
// calls.
const documentReferencesTimeout = 15 * time.Second

func (h *LangHandler) handleTextDocumentReferences(ctx context.Context, conn JSONRPC2Conn, req *jsonrpc2.Request, params lsp.ReferenceParams) ([]lsp.Location, error) {
	// TODO: Add support for the cancelRequest LSP method instead of using
	// hard-coded timeouts like this here.
	//
	// See: https://github.com/Microsoft/language-server-protocol/blob/master/protocol.md#cancelRequest
	ctx, cancel := context.WithTimeout(ctx, documentReferencesTimeout)
	defer cancel()

	fset, node, _, _, pkg, err := h.typecheck(ctx, conn, params.TextDocument.URI, params.Position)
	if err != nil {
		// Invalid nodes means we tried to click on something which is
		// not an ident (eg comment/string/etc). Return no information.
		if _, ok := err.(*invalidNodeError); ok {
			return []lsp.Location{}, nil
		}
		return nil, err
	}

	bctx := h.BuildContext(ctx)
	h.importGraphOnce.Do(func() {
		// We ignore the errors since we are doing a best-effort analysis
		_, rev, _ := importgraph.Build(bctx)
		h.importGraph = rev
	})

	// NOTICE: Code adapted from golang.org/x/tools/cmd/guru
	// referrers.go.

	obj := pkg.ObjectOf(node)
	if obj == nil {
		return nil, errors.New("references object not found")
	}

	// TODO(sqs): golang.org/x/tools/cmd/guru/referrers.go has some
	// other handling of obj == nil cases: type-switches, package
	// decls, and unresolved identifiers that we should adapt as well.
	if obj == nil {
		return nil, errors.New("object not found")
	}

	if obj.Pkg() == nil {
		if _, builtin := obj.(*types.Builtin); builtin {
			// We don't support builtin references due to the massive number
			// of references, so ignore the missing package error.
			return []lsp.Location{}, nil
		}
		return nil, fmt.Errorf("no package found for object %s", obj)
	}
	defpkg := obj.Pkg().Path()
	objposn := fset.Position(obj.Pos())
	_, pkgLevel := classify(obj)

	pkgInWorkspace := func(path string) bool {
		return PathHasPrefix(path, h.init.RootImportPath)
	}

	// Find the set of packages in this workspace that depend on
	// defpkg. Only function bodies in those packages need
	// type-checking.
	var users map[string]bool
	if pkgLevel {
		users = h.importGraph[defpkg]
		if users == nil {
			users = map[string]bool{}
		}
		users[defpkg] = true
	} else {
		users = h.importGraph.Search(defpkg)
	}
	lconf := loader.Config{
		Fset:  fset,
		Build: bctx,
		TypeCheckFuncBodies: func(path string) bool {
			if ctx.Err() != nil {
				return false
			}

			// Don't typecheck func bodies in dependency packages
			// (except the package that defines the object), because
			// we wouldn't return those refs anyway.
			path = strings.TrimSuffix(path, "_test")
			return users[path] && (pkgInWorkspace(path) || path == defpkg)
		},
	}
	allowErrors(&lconf)

	// The importgraph doesn't treat external test packages
	// as separate nodes, so we must use ImportWithTests.
	for path := range users {
		lconf.ImportWithTests(path)
	}

	// The remainder of this function is somewhat tricky because it
	// operates on the concurrent stream of packages observed by the
	// loader's AfterTypeCheck hook.

	var (
		mu                sync.Mutex
		refs              []*ast.Ident
		qobj              types.Object
		afterTypeCheckErr error
	)

	// For efficiency, we scan each package for references
	// just after it has been type-checked. The loader calls
	// AfterTypeCheck (concurrently), providing us with a stream of
	// packages.
	lconf.AfterTypeCheck = func(info *loader.PackageInfo, files []*ast.File) {
		// AfterTypeCheck may be called twice for the same package due
		// to augmentation.

		// Only inspect packages that depend on the declaring package
		// (and thus were type-checked).
		if lconf.TypeCheckFuncBodies(info.Pkg.Path()) {
			mu.Lock()
			defer mu.Unlock()

			// Record the query object and its package when we see
			// it. We can't reuse obj from the initial typecheck
			// because each go/loader Load invocation creates new
			// objects, and we need to test for equality later when we
			// look up refs.
			if qobj == nil && strings.TrimSuffix(info.Pkg.Path(), "_test") == defpkg {
				// Find the object by its position (slightly ugly).
				qobj = findObject(fset, &info.Info, objposn)
				if qobj == nil {
					// It really ought to be there; we found it once
					// already.
					afterTypeCheckErr = fmt.Errorf("object at %s not found in package %s", objposn, defpkg)
				}
			}
			obj := qobj

			// Look for references to the query object. Only collect
			// those that are in this workspace.
			if pkgInWorkspace(info.Pkg.Path()) {
				refs = append(refs, usesOf(obj, info)...)
			}
		}

		clearInfoFields(info) // save memory
	}

	done := make(chan struct{})
	go func() {
		// Prevent any uncaught panics from taking the entire server down.
		defer func() {
			close(done)
			_ = panicf(recover(), "%v", req.Method)
		}()

		lconf.Load() // ignore error
	}()

	// Wait for timeout or completion
	select {
	case <-done:
	case <-ctx.Done():
	}

	// We need to grab mu since it protects qobj
	mu.Lock()
	defer mu.Unlock()

	// If a timeout does occur, we should know how effective the partial data is
	if ctx.Err() != nil {
		refTimeoutResults.Observe(float64(len(refs)))
		log.Printf("info: timeout during references for %s, found %d refs", defpkg, len(refs))
	}

	if qobj == nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if afterTypeCheckErr != nil {
			// Only triggered by 1 specific error above (where we assign
			// afterTypeCheckErr), not any general loader error.
			return nil, afterTypeCheckErr
		}
		return nil, errors.New("query object not found during reloading")
	}

	// Don't include decl if it is outside of workspace.
	if params.Context.IncludeDeclaration && PathHasPrefix(defpkg, h.init.RootImportPath) {
		refs = append(refs, &ast.Ident{NamePos: obj.Pos(), Name: obj.Name()})
	}

	locs := goRangesToLSPLocations(fset, refs)
	sortBySharedDirWithURI(params.TextDocument.URI, locs)

	// Technically we may be able to stop computing references sooner and
	// save RAM/CPU, but currently that would have two drawbacks:
	// * We can't stop the typechecking anyways
	// * We may return results that are not as interesting since sortBySharedDirWithURI won't see everything.
	if params.Context.XLimit > 0 && params.Context.XLimit < len(locs) {
		locs = locs[:params.Context.XLimit]
	}

	return locs, nil
}

// classify classifies objects by how far
// we have to look to find references to them.
func classify(obj types.Object) (global, pkglevel bool) {
	if obj.Exported() {
		if obj.Parent() == nil {
			// selectable object (field or method)
			return true, false
		}
		if obj.Parent() == obj.Pkg().Scope() {
			// lexical object (package-level var/const/func/type)
			return true, true
		}
	}
	// object with unexported named or defined in local scope
	return false, false
}

// allowErrors causes type errors to be silently ignored.
// (Not suitable if SSA construction follows.)
//
// NOTICE: Adapted from golang.org/x/tools.
func allowErrors(lconf *loader.Config) {
	ctxt := *lconf.Build // copy
	ctxt.CgoEnabled = false
	lconf.Build = &ctxt
	lconf.AllowErrors = true
	// AllErrors makes the parser always return an AST instead of
	// bailing out after 10 errors and returning an empty ast.File.
	lconf.ParserMode = parser.AllErrors
	lconf.TypeChecker.Error = func(err error) {}
}

// findObject returns the object defined at the specified position.
func findObject(fset *token.FileSet, info *types.Info, objposn token.Position) types.Object {
	good := func(obj types.Object) bool {
		if obj == nil {
			return false
		}
		posn := fset.Position(obj.Pos())
		return posn.Filename == objposn.Filename && posn.Offset == objposn.Offset
	}
	for _, obj := range info.Defs {
		if good(obj) {
			return obj
		}
	}
	for _, obj := range info.Implicits {
		if good(obj) {
			return obj
		}
	}
	return nil
}

func usesOf(queryObj types.Object, info *loader.PackageInfo) []*ast.Ident {
	var refs []*ast.Ident
	for id, obj := range info.Uses {
		if sameObj(queryObj, obj) {
			refs = append(refs, id)
		}
	}
	return refs
}

// same reports whether x and y are identical, or both are PkgNames
// that import the same Package.
func sameObj(x, y types.Object) bool {
	if x == y {
		return true
	}
	if x, ok := x.(*types.PkgName); ok {
		if y, ok := y.(*types.PkgName); ok {
			return x.Imported() == y.Imported()
		}
	}
	return false
}

func sortBySharedDirWithURI(uri string, locs []lsp.Location) {
	l := locationList{
		L: locs,
		D: make([]int, len(locs)),
	}
	// l.D[i] = number of shared directories between uri and l.L[i].URI
	for i := range l.L {
		u := l.L[i].URI
		var d int
		for i := 0; i < len(uri) && i < len(u) && uri[i] == u[i]; i++ {
			if u[i] == '/' {
				d++
			}
		}
		if u == uri {
			// Boost matches in the same uri
			d++
		}
		l.D[i] = d
	}
	sort.Sort(l)
}

type locationList struct {
	L []lsp.Location
	D []int
}

func (l locationList) Less(a, b int) bool {
	if l.D[a] != l.D[b] {
		return l.D[a] > l.D[b]
	}
	if x, y := path.Dir(l.L[a].URI), path.Dir(l.L[b].URI); x != y {
		return x < y
	}
	if l.L[a].URI != l.L[b].URI {
		return l.L[a].URI < l.L[b].URI
	}
	if l.L[a].Range.Start.Line != l.L[b].Range.Start.Line {
		return l.L[a].Range.Start.Line < l.L[b].Range.Start.Line
	}
	return l.L[a].Range.Start.Character < l.L[b].Range.Start.Character
}

func (l locationList) Swap(a, b int) {
	l.L[a], l.L[b] = l.L[b], l.L[a]
	l.D[a], l.D[b] = l.D[b], l.D[a]
}
func (l locationList) Len() int {
	return len(l.L)
}

var refTimeoutResults = prometheus.NewHistogram(prometheus.HistogramOpts{
	Namespace: "golangserver",
	Subsystem: "references",
	Name:      "timeout_references",
	Help:      "The number of references that were returned after a timeout.",
	// 0.01 is to capture no results
	Buckets: []float64{0.01, 1, 2, 32, 128, 1024},
})

func init() {
	prometheus.MustRegister(refTimeoutResults)
}
