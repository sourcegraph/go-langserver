package godef

import (
	"bytes"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go/token"

	"go/printer"

	"go/ast"

	"github.com/lambdalab/go-langserver/langserver/internal/godef/go/parser"
	"github.com/lambdalab/go-langserver/langserver/internal/godef/go/types"
	"errors"
)

type Result struct {
	// Start and end positions of the definition (only if not an import statement).
	Start, End token.Pos

	// Package in question, only present if an import statement OR package selector
	// ('http' in 'http.Router').
	Package *build.Package
}

var ErrNoIdentifierFound = errors.New("no identifier found")

type packageCache struct {
	f map[string]*ast.File
	fset *token.FileSet
	scope *ast.Scope
}

var pkgCache = make(map[string]packageCache)
var pkgName = make(map[string]string)
var identMap = make(map[int]ast.Node)

var prevDirectory string = ""
var prevFilename string = ""

func clearCache(filename string) {
	d := filepath.Dir(filename)
	if  prevDirectory != d {
		prevDirectory = d
		pkgCache = make(map[string]packageCache)
		pkgName = make(map[string]string)
	}
}

func GetFilePackageName(filename string) string {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, filename, nil, parser.PackageClauseOnly, nil, types.DefaultImportPathToName)
	return f.Name.Name
}

func Godef(offset int, filename string, src []byte, fset **token.FileSet) (*Result, error) {
	var pkgScope *ast.Scope
	var err error
	var f *ast.File

	name, ok := pkgName[filename]
	if ok {
		pkgCacheHash := filepath.Dir(filename) + "/" + name
		*fset = pkgCache[pkgCacheHash].fset
		pkgScope = pkgCache[pkgCacheHash].scope
		f = pkgCache[pkgCacheHash].f[filename]
	} else {
		clearCache(filename)

		*fset = token.NewFileSet()
		pkgScope = ast.NewScope(parser.Universe)
		f, err = parser.ParseFile(*fset, filename, src, 0, pkgScope, types.DefaultImportPathToName)
		if f == nil {
			return nil, fmt.Errorf("cannot parse %s: %v", filename, err)
		}

		name := f.Name.Name
		pkgName[filename] = name

		pkgCacheHash := filepath.Dir(filename) + "/" + name
		pkgCache[pkgCacheHash] = packageCache{make(map[string]*ast.File),*fset,pkgScope}
		pkgCache[pkgCacheHash].f[filename] = f

		parseLocalPackage(*fset, filename, name, pkgScope, types.DefaultImportPathToName)
	}

	if prevFilename != filename {
		prevFilename = filename
		identMap = make(map[int]ast.Node)
		walkIdentifier(*fset, f, identMap)
	}

	o, ok := identMap[offset]
	if o == nil {
		return nil, ErrNoIdentifierFound
	}
	switch e := o.(type) {
	case *ast.ImportSpec:
		path, err := importPath(e)
		if err != nil {
			return nil, err
		}
		pkg, err := build.Default.Import(path, filepath.Dir(filename), build.FindOnly)
		if err != nil {
			return nil, fmt.Errorf("error finding import path for %s: %s", path, err)
		}
		return &Result{Package: pkg}, nil
	case ast.Expr:
		result := func(obj *ast.Object) (*Result, error) {
			p := types.DeclPos(obj)
			r := &Result{Start: p, End: p + token.Pos(len(obj.Name))}
			if imp, ok := obj.Decl.(*ast.ImportSpec); ok {
				path, err := importPath(imp)
				if err != nil {
					return nil, err
				}
				pkg, err := build.Default.Import(path, filepath.Dir((*fset).Position(p).Filename), build.FindOnly)
				if err != nil {
					return nil, fmt.Errorf("error finding import path for %s: %s", path, err)
				}
				r.Package = pkg
			}
			return r, nil
		}
		importer := types.DefaultImporter(*fset, pkgName[filename])

		if obj, _ := types.ExprType(e, importer, *fset); obj != nil {
			return result(obj)
		}

		return nil, fmt.Errorf("no declaration found for %v", pretty{*fset, e})
	}
	return nil, fmt.Errorf("unreached")
}

func importPath(n *ast.ImportSpec) (string, error) {
	p, err := strconv.Unquote(n.Path.Value)
	if err != nil {
		return "", fmt.Errorf("invalid string literal %q in ast.ImportSpec", n.Path.Value)
	}
	return p, nil
}

type visitorFunc func(n ast.Node) bool

func (f visitorFunc) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

func parseExpr(fset *token.FileSet, s *ast.Scope, expr string) (ast.Expr, error) {
	n, err := parser.ParseExpr(fset, "<arg>", expr, s, types.DefaultImportPathToName)
	if err != nil {
		return nil, fmt.Errorf("cannot parse expression: %v", err)
	}
	switch n := n.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return n, nil
	}
	return nil, fmt.Errorf("no identifier found in expression")
}

func walkIdentifier(fset *token.FileSet, f *ast.File, idMap map[int]ast.Node) {
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		var startPos token.Pos
		switch n := n.(type) {
		default:
			return true
		case *ast.Ident:
			startPos = n.NamePos
		case *ast.SelectorExpr:
			startPos = n.Sel.NamePos
		case *ast.ImportSpec:
			startPos = n.Pos()
		case *ast.StructType:
			for _, field := range n.Fields.List {
				if field.Names != nil {
					continue
				}
				t := field.Type
				if pt, ok := field.Type.(*ast.StarExpr); ok {
					t = pt.X
				}
				if id, ok := t.(*ast.Ident); ok {
					pos := fset.Position(id.NamePos).Offset
					e, err := parseExpr(fset, f.Scope, id.Name)
					if err != nil {
						log.Println(err) // TODO(slimsag): return to caller
					}
					idMap[pos] = e
				}
			}
			return true
		}
		pos := fset.Position(startPos).Offset
		_, ok := idMap[pos]
		if !ok {
			idMap[pos] = n
		}

		return true
	}
	ast.Walk(visitorFunc(visit), f)
}


// parseLocalPackage reads and parses all go files from the
// current directory that implement the same package name
// the principal source file, except the original source file
// itself, which will already have been parsed.
//
func parseLocalPackage(fset *token.FileSet, filename string, name string, pkgScope *ast.Scope, pathToName parser.ImportPathToName) error {
	d, f := filepath.Split(filename)
	if d == "" {
		d = "./"
	}
	fd, err := os.Open(d)
	if err != nil {
		return err
	}
	defer fd.Close()

	list, err := fd.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, pf := range list {
		file := filepath.Join(d, pf)
		if !strings.HasSuffix(pf, ".go") || pf == f {
			continue
		}
		nset := token.NewFileSet()
		src, _ := parser.ParseFile(nset, file, nil, parser.PackageClauseOnly, nil, pathToName)
		if src != nil && src.Name.Name == name {
			src, _ := parser.ParseFile(fset, file, nil, 0, pkgScope, pathToName)
			pkgCacheHash := filepath.Dir(filename) + "/" + name
			pkgCache[pkgCacheHash].f[file] = src
			pkgName[file] = name
		}
	}

	return nil
}

type pretty struct {
	fset *token.FileSet
	n    interface{}
}

func (p pretty) String() string {
	var b bytes.Buffer
	printer.Fprint(&b, p.fset, p.n)
	return b.String()
}
