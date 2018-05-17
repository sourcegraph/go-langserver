package langserver

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/buildutil"

	"github.com/neelance/parallel"
	"github.com/sourcegraph/go-langserver/langserver/util"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/go-langserver/pkg/lspext"
	"github.com/sourcegraph/go-langserver/pkg/tools"
	"github.com/sourcegraph/jsonrpc2"
)

// Query is a structured representation that is parsed from the user's
// raw query string.
type Query struct {
	Kind      lsp.SymbolKind
	Filter    FilterType
	File, Dir string
	Tokens    []string

	Symbol lspext.SymbolDescriptor
}

// String converts the query back into a logically equivalent, but not strictly
// byte-wise equal, query string. It is useful for converting a modified query
// structure back into a query string.
func (q Query) String() string {
	s := ""
	switch q.Filter {
	case FilterExported:
		s = queryJoin(s, "is:exported")
	case FilterDir:
		s = queryJoin(s, fmt.Sprintf("%s:%s", q.Filter, q.Dir))
	default:
		// no filter.
	}
	if q.Kind != 0 {
		for kwd, kind := range keywords {
			if kind == q.Kind {
				s = queryJoin(s, kwd)
			}
		}
	}
	for _, token := range q.Tokens {
		s = queryJoin(s, token)
	}
	return s
}

// queryJoin joins the strings into "<s><space><e>" ensuring there is no
// trailing or leading whitespace at the end of the string.
func queryJoin(s, e string) string {
	return strings.TrimSpace(s + " " + e)
}

// ParseQuery parses a user's raw query string and returns a
// structured representation of the query.
func ParseQuery(q string) (qu Query) {
	// All queries are case insensitive.
	q = strings.ToLower(q)

	// Split the query into space-delimited fields.
	for _, field := range strings.Fields(q) {
		// Check if the field is a filter like `is:exported`.
		if strings.HasPrefix(field, "dir:") {
			qu.Filter = FilterDir
			qu.Dir = strings.TrimPrefix(field, "dir:")
			continue
		}
		if field == "is:exported" {
			qu.Filter = FilterExported
			continue
		}

		// Each field is split into tokens, delimited by periods or slashes.
		tokens := strings.FieldsFunc(field, func(c rune) bool {
			return c == '.' || c == '/'
		})
		for _, tok := range tokens {
			if kind, isKeyword := keywords[tok]; isKeyword {
				qu.Kind = kind
				continue
			}
			qu.Tokens = append(qu.Tokens, tok)
		}
	}
	return qu
}

type FilterType string

const (
	FilterExported FilterType = "exported"
	FilterDir      FilterType = "dir"
)

// keywords are keyword tokens that will be interpreted as symbol kind
// filters in the search query.
var keywords = map[string]lsp.SymbolKind{
	"package": lsp.SKPackage,
	"type":    lsp.SKClass,
	"method":  lsp.SKMethod,
	"field":   lsp.SKField,
	"func":    lsp.SKFunction,
	"var":     lsp.SKVariable,
	"const":   lsp.SKConstant,
}

type symbolPair struct {
	lsp.SymbolInformation
	desc symbolDescriptor
}

// resultSorter is a utility struct for collecting, filtering, and
// sorting symbol results.
type resultSorter struct {
	Query
	results   []scoredSymbol
	resultsMu sync.Mutex
}

// scoredSymbol is a symbol with an attached search relevancy score.
// It is used internally by resultSorter.
type scoredSymbol struct {
	score int
	symbolPair
}

/*
 * sort.Interface methods
 */
func (s *resultSorter) Len() int { return len(s.results) }
func (s *resultSorter) Less(i, j int) bool {
	iscore, jscore := s.results[i].score, s.results[j].score
	if iscore == jscore {
		if s.results[i].ContainerName == s.results[j].ContainerName {
			if s.results[i].Name == s.results[j].Name {
				return s.results[i].Location.URI < s.results[j].Location.URI
			}
			return s.results[i].Name < s.results[j].Name
		}
		return s.results[i].ContainerName < s.results[j].ContainerName
	}
	return iscore > jscore
}
func (s *resultSorter) Swap(i, j int) {
	s.results[i], s.results[j] = s.results[j], s.results[i]
}

// Collect is a thread-safe method that will record the passed-in
// symbol in the list of results if its score > 0.
func (s *resultSorter) Collect(si symbolPair) {
	s.resultsMu.Lock()
	score := score(s.Query, si)
	if score > 0 {
		sc := scoredSymbol{score, si}
		s.results = append(s.results, sc)
	}
	s.resultsMu.Unlock()
}

// Results returns the ranked list of SymbolInformation values.
func (s *resultSorter) Results() []lsp.SymbolInformation {
	res := make([]lsp.SymbolInformation, len(s.results))
	for i, s := range s.results {
		res[i] = s.SymbolInformation
	}
	return res
}

// score returns 0 for results that aren't matches. Results that are matches are assigned
// a positive score, which should be used for ranking purposes.
func score(q Query, s symbolPair) (scor int) {
	if q.Kind != 0 {
		if q.Kind != s.Kind {
			return 0
		}
	}
	if q.Symbol != nil && !s.desc.Contains(q.Symbol) {
		return -1
	}
	name, container := strings.ToLower(s.Name), strings.ToLower(s.ContainerName)
	if !util.IsURI(s.Location.URI) {
		log.Printf("unexpectedly saw symbol defined at a non-file URI: %q", s.Location.URI)
		return 0
	}
	filename := util.UriToPath(s.Location.URI)
	isVendor := strings.HasPrefix(filename, "vendor/") || strings.Contains(filename, "/vendor/")
	if q.Filter == FilterExported && isVendor {
		// is:exported excludes vendor symbols always.
		return 0
	}
	if q.File != "" && filename != q.File {
		// We're restricting results to a single file, and this isn't it.
		return 0
	}
	if len(q.Tokens) == 0 { // early return if empty query
		if isVendor {
			return 1 // lower score for vendor symbols
		} else {
			return 2
		}
	}
	for i, tok := range q.Tokens {
		tok := strings.ToLower(tok)
		if strings.HasPrefix(container, tok) {
			scor += 2
		}
		if strings.HasPrefix(name, tok) {
			scor += 3
		}
		if strings.Contains(filename, tok) && len(tok) >= 3 {
			scor++
		}
		if strings.HasPrefix(path.Base(filename), tok) && len(tok) >= 3 {
			scor += 2
		}
		if tok == name {
			if i == len(q.Tokens)-1 {
				scor += 50
			} else {
				scor += 5
			}
		}
		if tok == container {
			scor += 3
		}
	}
	if scor > 0 && !(strings.HasPrefix(filename, "vendor/") || strings.Contains(filename, "/vendor/")) {
		// boost for non-vendor symbols
		scor += 5
	}
	if scor > 0 && ast.IsExported(s.Name) {
		// boost for exported symbols
		scor++
	}
	return scor
}

// toSym returns a SymbolInformation value derived from values we get
// from the Go parser and doc packages.
func toSym(name string, bpkg *build.Package, recv string, kind lsp.SymbolKind, fs *token.FileSet, pos token.Pos) symbolPair {
	var id string
	if recv == "" {
		id = fmt.Sprintf("%s/-/%s", path.Clean(bpkg.ImportPath), name)
	} else {
		id = fmt.Sprintf("%s/-/%s/%s", path.Clean(bpkg.ImportPath), recv, name)
	}

	return symbolPair{
		SymbolInformation: lsp.SymbolInformation{
			Name:          name,
			Kind:          kind,
			Location:      goRangeToLSPLocation(fs, pos, pos+token.Pos(len(name))),
			ContainerName: recv,
		},
		// NOTE: fields must be kept in sync with workspace_refs.go:defSymbolDescriptor
		desc: symbolDescriptor{
			Vendor:      util.IsVendorDir(bpkg.Dir),
			Package:     path.Clean(bpkg.ImportPath),
			PackageName: bpkg.Name,
			Recv:        recv,
			Name:        name,
			ID:          id,
		},
	}
}

// handleTextDocumentSymbol handles `textDocument/documentSymbol` requests for
// the Go language server.
func (h *LangHandler) handleTextDocumentSymbol(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.DocumentSymbolParams) ([]lsp.SymbolInformation, error) {
	if !util.IsURI(params.TextDocument.URI) {
		return nil, &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidParams,
			Message: fmt.Sprintf("textDocument/documentSymbol not yet supported for out-of-workspace URI (%q)", params.TextDocument.URI),
		}
	}
	path := util.UriToPath(params.TextDocument.URI)

	fset := token.NewFileSet()
	bctx := h.BuildContext(ctx)
	src, err := buildutil.ParseFile(fset, bctx, nil, filepath.Dir(path), filepath.Base(path), 0)
	if err != nil {
		return nil, err
	}
	pkg := &ast.Package{
		Name:  src.Name.Name,
		Files: map[string]*ast.File{},
	}
	pkg.Files[filepath.Base(path)] = src

	symbols := astPkgToSymbols(fset, pkg, &build.Package{})
	res := make([]lsp.SymbolInformation, len(symbols))
	for i, s := range symbols {
		res[i] = s.SymbolInformation
	}
	return res, nil
}

// handleSymbol handles `workspace/symbol` requests for the Go
// language server.
func (h *LangHandler) handleWorkspaceSymbol(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lspext.WorkspaceSymbolParams) ([]lsp.SymbolInformation, error) {
	q := ParseQuery(params.Query)
	q.Symbol = params.Symbol
	if q.Filter == FilterDir {
		q.Dir = path.Join(h.init.RootImportPath, q.Dir)
	}
	if id, ok := q.Symbol["id"]; ok {
		// id implicitly contains a dir hint. We can use that to
		// reduce the number of files we have to parse.
		q.Dir = strings.SplitN(id.(string), "/-/", 2)[0]
		q.Filter = FilterDir
	}
	if params.Limit == 0 {
		// If no limit is specified, default to a reasonable number
		// for a user to look at. If they want more, they should
		// refine the query.
		params.Limit = 50
	}
	return h.handleSymbol(ctx, conn, req, q, params.Limit)
}

func (h *LangHandler) handleSymbol(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, query Query, limit int) ([]lsp.SymbolInformation, error) {
	results := resultSorter{Query: query, results: make([]scoredSymbol, 0)}
	{
		rootPath := h.FilePath(h.init.Root())
		bctx := h.BuildContext(ctx)

		par := parallel.NewRun(h.config.MaxParallelism)
		for _, pkg := range tools.ListPkgsUnderDir(bctx, rootPath) {
			// If we're restricting results to a single file or dir, ensure the
			// package dir matches to avoid doing unnecessary work.
			if results.Query.File != "" {
				filePkgPath := path.Dir(results.Query.File)
				if util.PathHasPrefix(filePkgPath, bctx.GOROOT) {
					filePkgPath = util.PathTrimPrefix(filePkgPath, bctx.GOROOT)
				} else {
					filePkgPath = util.PathTrimPrefix(filePkgPath, bctx.GOPATH)
				}
				filePkgPath = util.PathTrimPrefix(filePkgPath, "src")
				if !util.PathEqual(pkg, filePkgPath) {
					continue
				}
			}
			if results.Query.Filter == FilterDir && !util.PathEqual(pkg, results.Query.Dir) {
				continue
			}

			par.Acquire()

			// If the context is cancelled, breaking the loop here
			// will allow us to return partial results, and
			// avoiding starting new computations.
			if ctx.Err() != nil {
				par.Release()
				break
			}

			go func(pkg string) {
				// Prevent any uncaught panics from taking the
				// entire server down. For an example see
				// https://github.com/golang/go/issues/17788
				defer func() {
					par.Release()
					_ = util.Panicf(recover(), "%v for pkg %v", req.Method, pkg)
				}()
				h.collectFromPkg(ctx, bctx, pkg, rootPath, &results)
			}(pkg)
		}
		_ = par.Wait()
	}
	sort.Sort(&results)
	if len(results.results) > limit && limit > 0 {
		results.results = results.results[:limit]
	}

	return results.Results(), nil
}

type pkgSymResult struct {
	ready   chan struct{} // closed to broadcast readiness
	symbols []lsp.SymbolInformation
}

// collectFromPkg collects all the symbols from the specified package
// into the results. It uses LangHandler's package symbol cache to
// speed up repeated calls.
func (h *LangHandler) collectFromPkg(ctx context.Context, bctx *build.Context, pkg string, rootPath string, results *resultSorter) {
	symbols := h.symbolCache.Get(pkg, func() interface{} {
		findPackage := h.getFindPackageFunc()
		buildPkg, err := findPackage(ctx, bctx, pkg, rootPath, 0)
		if err != nil {
			maybeLogImportError(pkg, err)
			return nil
		}

		fs := token.NewFileSet()
		astPkgs, err := parseDir(fs, bctx, buildPkg.Dir, nil, 0)
		if err != nil {
			log.Printf("failed to parse directory %s: %s", buildPkg.Dir, err)
			return nil
		}
		astPkg := astPkgs[buildPkg.Name]
		if astPkg == nil {
			return nil
		}

		return astPkgToSymbols(fs, astPkg, buildPkg)
	})

	if symbols == nil {
		return
	}

	for _, sym := range symbols.([]symbolPair) {
		if results.Query.Filter == FilterExported && !isExported(&sym) {
			continue
		}
		results.Collect(sym)
	}
}

// astToSymbols returns a slice of LSP symbols from an AST.
func astPkgToSymbols(fs *token.FileSet, astPkg *ast.Package, buildPkg *build.Package) []symbolPair {
	// TODO(keegancsmith) Remove vendored doc/go once https://github.com/golang/go/issues/17788 is shipped
	docPkg := doc.New(astPkg, buildPkg.ImportPath, doc.AllDecls)

	// Emit decls
	var pkgSyms []symbolPair
	for _, t := range docPkg.Types {
		pkgSyms = append(pkgSyms, toSym(t.Name, buildPkg, "", typeSpecSym(t), fs, declNamePos(t.Decl, t.Name)))
		for _, v := range t.Funcs {
			pkgSyms = append(pkgSyms, toSym(v.Name, buildPkg, "", lsp.SKFunction, fs, v.Decl.Name.NamePos))
		}
		for _, v := range t.Methods {
			pkgSyms = append(pkgSyms, toSym(v.Name, buildPkg, t.Name, lsp.SKMethod, fs, v.Decl.Name.NamePos))
		}
		for _, v := range t.Consts {
			for _, name := range v.Names {
				pkgSyms = append(pkgSyms, toSym(name, buildPkg, "", lsp.SKConstant, fs, declNamePos(v.Decl, name)))
			}
		}
		for _, v := range t.Vars {
			for _, name := range v.Names {
				pkgSyms = append(pkgSyms, toSym(name, buildPkg, "", lsp.SKField, fs, declNamePos(v.Decl, name)))
			}
		}
	}
	for _, v := range docPkg.Consts {
		for _, name := range v.Names {
			pkgSyms = append(pkgSyms, toSym(name, buildPkg, "", lsp.SKConstant, fs, declNamePos(v.Decl, name)))
		}
	}
	for _, v := range docPkg.Vars {
		for _, name := range v.Names {
			pkgSyms = append(pkgSyms, toSym(name, buildPkg, "", lsp.SKVariable, fs, declNamePos(v.Decl, name)))
		}
	}
	for _, v := range docPkg.Funcs {
		pkgSyms = append(pkgSyms, toSym(v.Name, buildPkg, "", lsp.SKFunction, fs, v.Decl.Name.NamePos))
	}

	return pkgSyms
}

func typeSpecSym(t *doc.Type) lsp.SymbolKind {
	// This usually has one, but we're running through a for loop in case it has
	// none. In either case, the default is an SKClass.

	// NOTE: coincidentally, doing this gives access to the methods for an
	// interface and fields for a struct. Possible solution to Github issue #36?
	for _, s := range t.Decl.Specs {
		if v, ok := s.(*ast.TypeSpec); ok {
			if _, ok := v.Type.(*ast.InterfaceType); ok {
				return lsp.SKInterface
			}
		}
	}

	return lsp.SKClass
}

func declNamePos(decl *ast.GenDecl, name string) token.Pos {
	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.ImportSpec:
			if spec.Name != nil {
				return spec.Name.Pos()
			}
			return spec.Path.Pos()
		case *ast.ValueSpec:
			for _, specName := range spec.Names {
				if specName.Name == name {
					return specName.NamePos
				}
			}
		case *ast.TypeSpec:
			return spec.Name.Pos()
		}
	}
	return decl.TokPos
}

// parseDir mirrors parser.ParseDir, but uses the passed in build context's VFS. In other words,
// buildutil.parseFile : parser.ParseFile :: parseDir : parser.ParseDir
func parseDir(fset *token.FileSet, bctx *build.Context, path string, filter func(os.FileInfo) bool, mode parser.Mode) (pkgs map[string]*ast.Package, first error) {
	list, err := buildutil.ReadDir(bctx, path)
	if err != nil {
		return nil, err
	}

	pkgs = map[string]*ast.Package{}
	for _, d := range list {
		if strings.HasSuffix(d.Name(), ".go") && (filter == nil || filter(d)) {
			filename := buildutil.JoinPath(bctx, path, d.Name())
			if src, err := buildutil.ParseFile(fset, bctx, nil, buildutil.JoinPath(bctx, path, d.Name()), filename, mode); err == nil {
				name := src.Name.Name
				pkg, found := pkgs[name]
				if !found {
					pkg = &ast.Package{
						Name:  name,
						Files: map[string]*ast.File{},
					}
					pkgs[name] = pkg
				}
				pkg.Files[filename] = src
			} else if first == nil {
				first = err
			}
		}
	}

	return
}

func isExported(sym *symbolPair) bool {
	if sym.ContainerName == "" {
		return ast.IsExported(sym.Name)
	}
	return ast.IsExported(sym.ContainerName) && ast.IsExported(sym.Name)
}

func maybeLogImportError(pkg string, err error) {
	_, isNoGoError := err.(*build.NoGoError)
	if !(isNoGoError || !isMultiplePackageError(err) || strings.HasPrefix(pkg, "github.com/golang/go/test/")) {
		log.Printf("skipping possible package %s: %s", pkg, err)
	}
}
