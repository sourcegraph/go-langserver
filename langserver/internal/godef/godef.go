package godef

import (
	"bytes"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go/token"

	"go/printer"

	"go/ast"

	"github.com/lambdalab/go-langserver/langserver/internal/godef/go/parser"
	"github.com/lambdalab/go-langserver/langserver/internal/godef/go/types"
)

type Result struct {
	// Start and end positions of the definition (only if not an import statement).
	Start, End token.Pos

	// Package in question, only present if an import statement OR package selector
	// ('http' in 'http.Router').
	Package *build.Package
}

type PackageCache struct {
	f map[string]*ast.File
	fset *token.FileSet
	scope *ast.Scope
}

var pkgCache map[string]PackageCache
var pkgName  map[string]string
var identMap map[int]ast.Node

var prevDirectory string = ""
var prevFilename string = ""

func ClearCache(filename string) {
	d := filepath.Dir(filename)
	if  prevDirectory != d {
		prevDirectory = d
		pkgCache = make(map[string]PackageCache)
		pkgName = make(map[string]string)
	}
}

func Godef(offset int, filename string, src []byte, fset **token.FileSet) (*Result, error) {
	var pkgScope *ast.Scope
	var err error
	var f *ast.File

	name, ok := pkgName[filename]
	if ok {
		*fset = pkgCache[name].fset
		pkgScope = pkgCache[name].scope
		f = pkgCache[name].f[filename]
	} else {
		ClearCache(filename)

		*fset = token.NewFileSet()
		pkgScope = ast.NewScope(parser.Universe)
		f, err = parser.ParseFile(*fset, filename, src, 0, pkgScope, types.DefaultImportPathToName)
		if f == nil {
			return nil, fmt.Errorf("cannot parse %s: %v", filename, err)
		}

		name := f.Name.Name
		pkgName[filename] = name

		pkgCache[name] = PackageCache{make(map[string]*ast.File),*fset,pkgScope}
		pkgCache[name].f[filename] = f

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
		importer := types.DefaultImporter(*fset)

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

// findIdentifier looks for an identifier at byte-offset searchpos
// inside the parsed source represented by node.
// If it is part of a selector expression, it returns
// that expression rather than the identifier itself.
//
// As a special case, if it finds an import
// spec, it returns ImportSpec.
//
func findIdentifier(fset *token.FileSet, f *ast.File, searchpos int) ast.Node {
	ec := make(chan ast.Node)
	found := func(startPos, endPos token.Pos) bool {
		start := fset.Position(startPos).Offset
		end := start + int(endPos-startPos)
		return start <= searchpos && searchpos <= end
	}
	go func() {
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
				// If we find an anonymous bare field in a
				// struct type, its definition points to itself,
				// but we actually want to go elsewhere,
				// so assume (dubiously) that the expression
				// works globally and return a new node for it.
				for _, field := range n.Fields.List {
					if field.Names != nil {
						continue
					}
					t := field.Type
					if pt, ok := field.Type.(*ast.StarExpr); ok {
						t = pt.X
					}
					if id, ok := t.(*ast.Ident); ok {
						if found(id.NamePos, id.End()) {
							e, err := parseExpr(fset, f.Scope, id.Name)
							if err != nil {
								log.Println(err) // TODO(slimsag): return to caller
							}
							ec <- e
							runtime.Goexit()
						}
					}
				}
				return true
			}
			if found(startPos, n.End()) {
				ec <- n
				runtime.Goexit()
			}
			return true
		}
		ast.Walk(visitorFunc(visit), f)
		ec <- nil
	}()
	return <-ec
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

type visitorFunc func(n ast.Node) bool

func (f visitorFunc) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
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
		src, _ := parser.ParseFile(nset, filename, nil, parser.PackageClauseOnly, nil, pathToName)
		if src != nil && src.Name.Name == name {
			src, _ := parser.ParseFile(fset, file, nil, 0, pkgScope, pathToName)
			pkgCache[name].f[file] = src
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
