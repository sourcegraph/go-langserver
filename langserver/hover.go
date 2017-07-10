package langserver

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	doc "github.com/slimsag/godocmd"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *LangHandler) handleHover(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request, params lsp.TextDocumentPositionParams) (*lsp.Hover, error) {
	bctx := h.BuildContext(ctx)

	// First perform the equivalent of a textDocument/definition request in
	// order to resolve the definition position.
	fset, res, _, err := h.definitionGodef(ctx, params)
	if err != nil {
		return nil, err
	}

	// If our target is a package import statement or package selector, then we
	// handle that separately now.
	if res.Package != nil {
		// res.Package.Name is invalid since it was imported with FindOnly, so
		// import normally now.
		findPackage := h.getFindPackageFunc()
		bpkg, err := findPackage(ctx, bctx, res.Package.ImportPath, res.Package.Dir, 0)
		if err != nil {
			return nil, err
		}

		// Parse the entire dir into its respective AST packages.
		pkgs, err := parseDir(fset, bctx, res.Package.Dir, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		pkg := pkgs[bpkg.Name]

		// Find the package doc comments.
		pkgFiles := make([]*ast.File, 0, len(pkg.Files))
		for _, f := range pkg.Files {
			pkgFiles = append(pkgFiles, f)
		}
		comments := packageDoc(pkgFiles, bpkg.Name)

		return &lsp.Hover{
			Contents: maybeAddComments(comments, []lsp.MarkedString{{Language: "go", Value: fmt.Sprintf("package %s (%q)", bpkg.Name, bpkg.ImportPath)}}),

			// TODO(slimsag): I think we can add Range here, but not exactly
			// sure. res.Start and res.End are only present if it's a package
			// selector, not an import statement. Since Range is optional,
			// we're omitting it here.
		}, nil
	}

	loc := goRangeToLSPLocation(fset, res.Start, res.End)

	if loc.URI == "file://" {
		// TODO: builtins do not have valid URIs or locations.
		return &lsp.Hover{}, nil
	}

	filename := uriToFilePath(loc.URI)

	// Parse the entire dir into its respective AST packages.
	pkgs, err := parseDir(fset, bctx, filepath.Dir(filename), nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Locate the AST package that contains the file we're interested in.
	foundImportPath, foundPackage, err := packageForFile(pkgs, filename)
	if err != nil {
		return nil, err
	}

	// Create documentation for the package.
	docPkg := doc.New(foundPackage, foundImportPath, doc.AllDecls)

	// Locate the target in the docs.
	target := fset.Position(res.Start)
	docObject := findDocTarget(fset, target, docPkg)
	if docObject == nil {
		return nil, fmt.Errorf("failed to find doc object for %s", target)
	}

	contents, node := fmtDocObject(fset, docObject, target)
	r := rangeForNode(fset, node)
	return &lsp.Hover{
		Contents: contents,
		Range:    &r,
	}, nil
}

// maybeAddComments appends the specified comments converted to Markdown godoc
// form to the specified contents slice, if the comments string is not empty.
func maybeAddComments(comments string, contents []lsp.MarkedString) []lsp.MarkedString {
	if comments == "" {
		return contents
	}
	var b bytes.Buffer
	doc.ToMarkdown(&b, comments, nil)
	return append(contents, lsp.RawMarkedString(b.String()))
}

// packageDoc finds the documentation for the named package from its files or
// additional files.
func packageDoc(files []*ast.File, pkgName string) string {
	for _, f := range files {
		if f.Name.Name == pkgName {
			txt := f.Doc.Text()
			if strings.TrimSpace(txt) != "" {
				return txt
			}
		}
	}
	return ""
}

// packageForFile returns the import path and pkg from pkgs that contains the
// named file.
func packageForFile(pkgs map[string]*ast.Package, filename string) (string, *ast.Package, error) {
	for path, pkg := range pkgs {
		for pkgFile := range pkg.Files {
			if pkgFile == filename {
				return path, pkg, nil
			}
		}
	}
	return "", nil, fmt.Errorf("failed to find %q in packages %q", filename, pkgs)
}

// inRange tells if x is in the range of a-b inclusive.
func inRange(x, a, b token.Position) bool {
	if x.Filename != a.Filename || x.Filename != b.Filename {
		return false
	}
	return x.Offset >= a.Offset && x.Offset <= b.Offset
}

// findDocTarget walks an input *doc.Package and locates the *doc.Value,
// *doc.Type, or *doc.Func for the given target position.
func findDocTarget(fset *token.FileSet, target token.Position, in interface{}) interface{} {
	switch v := in.(type) {
	case *doc.Package:
		for _, x := range v.Consts {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Types {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Vars {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Funcs {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		return nil
	case *doc.Value:
		if inRange(target, fset.Position(v.Decl.Pos()), fset.Position(v.Decl.End())) {
			return v
		}
		return nil
	case *doc.Type:
		if inRange(target, fset.Position(v.Decl.Pos()), fset.Position(v.Decl.End())) {
			return v
		}

		for _, x := range v.Consts {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Vars {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Funcs {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		for _, x := range v.Methods {
			if r := findDocTarget(fset, target, x); r != nil {
				return r
			}
		}
		return nil
	case *doc.Func:
		if inRange(target, fset.Position(v.Decl.Pos()), fset.Position(v.Decl.End())) {
			return v
		}
		return nil
	default:
		panic("unreachable")
	}
}

// fmtDocObject formats one of:
//
// *doc.Value
// *doc.Type
// *doc.Func
//
func fmtDocObject(fset *token.FileSet, x interface{}, target token.Position) ([]lsp.MarkedString, ast.Node) {
	switch v := x.(type) {
	case *doc.Value: // Vars and Consts
		// Sort the specs by distance to find the one nearest to target.
		sort.Sort(byDistance{v.Decl.Specs, fset, target})
		spec := v.Decl.Specs[0].(*ast.ValueSpec)

		// Use the doc directly above the var inside a var() block, or if there
		// is none, fall back to the doc directly above the var() block.
		doc := spec.Doc.Text()
		if doc == "" {
			doc = v.Doc
		}

		// Create a copy of the spec with no doc for formatting separately.
		cpy := *spec
		cpy.Doc = nil
		value := v.Decl.Tok.String() + " " + fmtNode(fset, &cpy)
		return maybeAddComments(doc, []lsp.MarkedString{{Language: "go", Value: value}}), spec

	case *doc.Type: // Type declarations
		spec := v.Decl.Specs[0].(*ast.TypeSpec)

		// Handle interfaces methods and struct fields separately now.
		switch s := spec.Type.(type) {
		case *ast.InterfaceType:
			// Find the method that is an exact match for our target position.
			for _, field := range s.Methods.List {
				if fset.Position(field.Pos()).Offset == target.Offset {
					// An exact match.
					value := fmt.Sprintf("func (%s).%s%s", spec.Name.Name, field.Names[0].Name, strings.TrimPrefix(fmtNode(fset, field.Type), "func"))
					return maybeAddComments(field.Doc.Text(), []lsp.MarkedString{{Language: "go", Value: value}}), field
				}
			}

		case *ast.StructType:
			// Find the field that is an exact match for our target position.
			for _, field := range s.Fields.List {
				if fset.Position(field.Pos()).Offset == target.Offset {
					// An exact match.
					value := fmt.Sprintf("struct field %s %s", field.Names[0], fmtNode(fset, field.Type))
					return maybeAddComments(field.Doc.Text(), []lsp.MarkedString{{Language: "go", Value: value}}), field
				}
			}
		}

		// Formatting of all type declarations: structs, interfaces, integers, etc.
		name := v.Decl.Tok.String() + " " + spec.Name.Name + " " + typeName(fset, spec.Type)
		res := []lsp.MarkedString{{Language: "go", Value: name}}

		doc := spec.Doc.Text()
		if doc == "" {
			doc = v.Doc
		}
		res = maybeAddComments(doc, res)

		if n := typeName(fset, spec.Type); n == "interface" || n == "struct" {
			res = append(res, lsp.MarkedString{Language: "go", Value: fmtNode(fset, spec.Type)})
		}
		return res, spec

	case *doc.Func: // Functions
		return maybeAddComments(v.Doc, []lsp.MarkedString{{Language: "go", Value: fmtNode(fset, v.Decl)}}), v.Decl
	default:
		panic("unreachable")
	}
}

// typeName returns the name of typ, shortening interface and struct types to
// just "interface" and "struct" rather than their full contents (incl. methods
// and fields).
func typeName(fset *token.FileSet, typ ast.Expr) string {
	switch typ.(type) {
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		return "struct"
	default:
		return fmtNode(fset, typ)
	}
}

// fmtNode formats the given node as a string.
func fmtNode(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	err := format.Node(&buf, fset, n)
	if err != nil {
		panic("unreachable")
	}
	return buf.String()
}

// byDistance sorts specs by distance to the target position.
type byDistance struct {
	specs  []ast.Spec
	fset   *token.FileSet
	target token.Position
}

func (b byDistance) Len() int      { return len(b.specs) }
func (b byDistance) Swap(i, j int) { b.specs[i], b.specs[j] = b.specs[j], b.specs[i] }
func (b byDistance) Less(ii, jj int) bool {
	i := b.fset.Position(b.specs[ii].Pos())
	j := b.fset.Position(b.specs[jj].Pos())
	return abs(b.target.Offset-i.Offset) < abs(b.target.Offset-j.Offset)
}

// abs returns the absolute value of x.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
