package refs

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
	"testing"

	_ "gopkg.in/inconshreveable/log15.v2"
)

func testConfig(fs *token.FileSet, pkgName string, files []*ast.File) *Config {
	info := types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Implicits:  map[ast.Node]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	cfg := &types.Config{
		Importer:                 importer.Default(),
		FakeImportC:              true,
		DisableUnusedImportCheck: true,
		Error: func(error) {},
	}
	pkg, err := cfg.Check(pkgName, fs, files, &info)
	if err != nil {
		panic(err)
	}
	return &Config{
		FileSet:  fs,
		Pkg:      pkg,
		PkgFiles: files,
		Info:     &info,
	}
}

func TestParseFile(t *testing.T) {
	pos := func(s string) token.Position {
		f := strings.Fields(s)
		fp := strings.Split(f[0], ":")
		line, err := strconv.Atoi(fp[1])
		if err != nil {
			panic(err)
		}
		column, err := strconv.Atoi(fp[2])
		if err != nil {
			panic(err)
		}
		offs, err := strconv.Atoi(strings.TrimSuffix(f[2], ")"))
		if err != nil {
			panic(err)
		}
		return token.Position{
			Filename: fp[0],
			Line:     line,
			Column:   column,
			Offset:   offs,
		}
	}
	cases := []struct {
		Filename string
		Want     []*Ref
	}{
		{
			Filename: "testdata/empty.go",
			Want:     nil,
		},
		{
			Filename: "testdata/imports.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Position: pos("testdata/imports.go:3:8 (offset 21)")},
			},
		},
		{
			Filename: "testdata/unmatching-imports.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "gopkg.in/inconshreveable/log15.v2", PackageName: "log15", Path: ""}, Position: pos("testdata/unmatching-imports.go:3:8 (offset 21)")},
				&Ref{Def: Def{ImportPath: "gopkg.in/inconshreveable/log15.v2", PackageName: "log15", Path: "Crit"}, Position: pos("testdata/unmatching-imports.go:6:8 (offset 124)")},
			},
		},
		{
			Filename: "testdata/http-request-headers.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Position: pos("testdata/http-request-headers.go:4:2 (offset 24)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request"}, Position: pos("testdata/http-request-headers.go:8:13 (offset 78)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request Header"}, Position: pos("testdata/http-request-headers.go:9:4 (offset 113)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request Header"}, Position: pos("testdata/http-request-headers.go:11:4 (offset 172)")},
			},
		},
		{
			Filename: "testdata/convoluted.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Position: pos("testdata/convoluted.go:3:8 (offset 21)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/convoluted.go:6:10 (offset 84)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper RoundTrip"}, Position: pos("testdata/convoluted.go:15:14 (offset 188)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Position: pos("testdata/convoluted.go:15:4 (offset 178)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper RoundTrip"}, Position: pos("testdata/convoluted.go:19:25 (offset 310)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Position: pos("testdata/convoluted.go:19:15 (offset 300)")},
			},
		},
		{
			Filename: "testdata/defs.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Position: pos("testdata/defs.go:3:8 (offset 21)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/defs.go:6:10 (offset 78)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Position: pos("testdata/defs.go:7:9 (offset 119)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/defs.go:10:21 (offset 182)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/defs.go:12:12 (offset 246)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Position: pos("testdata/defs.go:13:11 (offset 295)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/defs.go:20:13 (offset 392)")},
			},
		},
		{
			Filename: "testdata/vars.go",
			Want: []*Ref{
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Position: pos("testdata/vars.go:3:8 (offset 21)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/vars.go:6:14 (offset 74)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Position: pos("testdata/vars.go:8:12 (offset 124)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Position: pos("testdata/vars.go:12:3 (offset 225)")},
				&Ref{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Position: pos("testdata/vars.go:11:12 (offset 194)")},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.Filename, func(t *testing.T) {
			cont, err := ioutil.ReadFile(c.Filename)
			if err != nil {
				t.Fatal(err)
			}
			fs := token.NewFileSet()
			astFile, err := parser.ParseFile(fs, c.Filename, cont, 0)
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig(fs, "refstest", []*ast.File{astFile})
			var allRefs []*Ref
			err = cfg.Refs(func(r *Ref) {
				allRefs = append(allRefs, r)
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(allRefs) != len(c.Want) {
				t.Log("got", len(allRefs), "refs:")
				for i, r := range allRefs {
					t.Logf("    %d. %+v\n", i, r)
				}
				t.Log("want", len(c.Want), "refs:")
				for i, r := range c.Want {
					t.Logf("    %d. %+v\n", i, r)
				}
				t.FailNow()
			}
			for i, ref := range allRefs {
				if !reflect.DeepEqual(ref, c.Want[i]) {
					t.Log("got", len(allRefs), "refs:")
					for i, r := range allRefs {
						t.Logf("    %d. %+v\n", i, r)
					}
					t.Log("want", len(c.Want), "refs:")
					for i, r := range c.Want {
						t.Logf("    %d. %+v\n", i, r)
					}
					t.FailNow()
				}
			}
		})
	}
}
