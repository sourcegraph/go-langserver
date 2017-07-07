package godef

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"

	"github.com/sourcegraph/ctxvfs"
	"github.com/sourcegraph/go-langserver/langserver/internal/godef/go/parser"
)

// This file contains mostly copied functions from ./go/parser/interface.go but
// modified to use only ctxvfs instead of os filesystem. We can't use the same
// implementation we use in e.g. functions like /langserver/symbol.go:parseDir
// because the ./go/parser signatures differ quite a lot.
//
// TODO: To remove this, we'll have to replace ./go/parser with stdlib
// go/parser outright.

func readSource(ctx context.Context, fs ctxvfs.FileSystem, filename string, src interface{}) ([]byte, error) {
	if src != nil {
		switch s := src.(type) {
		case string:
			return []byte(s), nil
		case []byte:
			return s, nil
		case *bytes.Buffer:
			// is io.Reader, but src is already available in []byte form
			if s != nil {
				return s.Bytes(), nil
			}
		case io.Reader:
			var buf bytes.Buffer
			_, err := io.Copy(&buf, s)
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		default:
			return nil, errors.New("invalid source")
		}
	}

	return ctxvfs.ReadFile(ctx, fs, filename)
}

func parseFile(ctx context.Context, fs ctxvfs.FileSystem, fset *token.FileSet, filename string, src interface{}, mode uint, pkgScope *ast.Scope, pathToName parser.ImportPathToName) (*ast.File, error) {
	src, err := ctxvfs.ReadFile(ctx, fs, filename)
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(fset, filename, src, mode, pkgScope, pathToName)
}

func parseFileInPkg(ctx context.Context, fs ctxvfs.FileSystem, fset *token.FileSet, pkgs map[string]*ast.Package, filename string, mode uint, pathToName parser.ImportPathToName) (err error) {
	data, err := readSource(ctx, fs, filename, nil)
	if err != nil {
		return err
	}
	// first find package name, so we can use the correct package
	// scope when parsing the file.
	src, err := parseFile(ctx, fs, fset, filename, data, parser.PackageClauseOnly, nil, pathToName)
	if err != nil {
		return
	}
	name := src.Name.Name
	pkg := pkgs[name]
	if pkg == nil {
		pkg = &ast.Package{name, ast.NewScope(parser.Universe), nil, make(map[string]*ast.File)}
		pkgs[name] = pkg
	}
	src, err = parseFile(ctx, fs, fset, filename, data, mode, pkg.Scope, pathToName)
	if err != nil {
		return
	}
	pkg.Files[filename] = src
	return
}

func parseFiles(ctx context.Context, fs ctxvfs.FileSystem, fset *token.FileSet, filenames []string, mode uint, pathToName parser.ImportPathToName) (pkgs map[string]*ast.Package, first error) {
	pkgs = make(map[string]*ast.Package)
	for _, filename := range filenames {
		if err := parseFileInPkg(ctx, fs, fset, pkgs, filename, mode, pathToName); err != nil && first == nil {
			first = err
		}
	}
	return
}

func parseDir(ctx context.Context, fs ctxvfs.FileSystem, fset *token.FileSet, path string, filter func(os.FileInfo) bool, mode uint, pathToName parser.ImportPathToName) (map[string]*ast.Package, error) {
	list, err := fs.ReadDir(ctx, path)
	if err != nil {
		return nil, err
	}

	filenames := make([]string, len(list))
	n := 0
	for i := 0; i < len(list); i++ {
		d := list[i]
		if filter == nil || filter(d) {
			filenames[n] = filepath.Join(path, d.Name())
			n++
		}
	}
	filenames = filenames[0:n]

	return parseFiles(ctx, fs, fset, filenames, mode, pathToName)
}
