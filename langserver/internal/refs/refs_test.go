package refs

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
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
		Error:                    func(error) {},
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
	type posRef struct {
		Def        Def
		Start, End token.Position
	}
	cases := []struct {
		Filename string
		Want     []*posRef
	}{
		{
			Filename: "testdata/empty.go",
			Want:     nil,
		},
		{
			Filename: "testdata/imports.go",
			Want: []*posRef{
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Start: pos("testdata/imports.go:3:8 (offset 21)"), End: pos("testdata/imports.go:3:20 (offset 33)")},
			},
		},
		{
			Filename: "testdata/http-request-headers.go",
			Want: []*posRef{
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Start: pos("testdata/http-request-headers.go:4:2 (offset 24)"), End: pos("testdata/http-request-headers.go:4:12 (offset 34)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request"}, Start: pos("testdata/http-request-headers.go:8:13 (offset 78)"), End: pos("testdata/http-request-headers.go:8:20 (offset 85)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request Header"}, Start: pos("testdata/http-request-headers.go:9:4 (offset 113)"), End: pos("testdata/http-request-headers.go:9:10 (offset 119)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Request Header"}, Start: pos("testdata/http-request-headers.go:11:4 (offset 172)"), End: pos("testdata/http-request-headers.go:11:10 (offset 178)")},
			},
		},
		{
			Filename: "testdata/convoluted.go",
			Want: []*posRef{
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Start: pos("testdata/convoluted.go:3:8 (offset 21)"), End: pos("testdata/convoluted.go:3:25 (offset 38)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/convoluted.go:6:10 (offset 84)"), End: pos("testdata/convoluted.go:6:16 (offset 90)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper RoundTrip"}, Start: pos("testdata/convoluted.go:15:14 (offset 188)"), End: pos("testdata/convoluted.go:15:23 (offset 197)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Start: pos("testdata/convoluted.go:15:4 (offset 178)"), End: pos("testdata/convoluted.go:15:13 (offset 187)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper RoundTrip"}, Start: pos("testdata/convoluted.go:19:25 (offset 310)"), End: pos("testdata/convoluted.go:19:34 (offset 319)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Start: pos("testdata/convoluted.go:19:15 (offset 300)"), End: pos("testdata/convoluted.go:19:24 (offset 309)")},
			},
		},
		{
			Filename: "testdata/defs.go",
			Want: []*posRef{
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Start: pos("testdata/defs.go:3:8 (offset 21)"), End: pos("testdata/defs.go:3:18 (offset 31)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/defs.go:6:10 (offset 78)"), End: pos("testdata/defs.go:6:16 (offset 84)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Start: pos("testdata/defs.go:7:9 (offset 119)"), End: pos("testdata/defs.go:7:21 (offset 131)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/defs.go:10:21 (offset 182)"), End: pos("testdata/defs.go:10:27 (offset 188)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/defs.go:12:12 (offset 246)"), End: pos("testdata/defs.go:12:18 (offset 252)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Start: pos("testdata/defs.go:13:11 (offset 295)"), End: pos("testdata/defs.go:13:23 (offset 307)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/defs.go:20:13 (offset 392)"), End: pos("testdata/defs.go:20:19 (offset 398)")},
			},
		},
		{
			Filename: "testdata/vars.go",
			Want: []*posRef{
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: ""}, Start: pos("testdata/vars.go:3:8 (offset 21)"), End: pos("testdata/vars.go:3:18 (offset 31)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/vars.go:6:14 (offset 74)"), End: pos("testdata/vars.go:6:20 (offset 80)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "RoundTripper"}, Start: pos("testdata/vars.go:8:12 (offset 124)"), End: pos("testdata/vars.go:8:24 (offset 136)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client Transport"}, Start: pos("testdata/vars.go:12:3 (offset 225)"), End: pos("testdata/vars.go:12:12 (offset 234)")},
				{Def: Def{ImportPath: "net/http", PackageName: "http", Path: "Client"}, Start: pos("testdata/vars.go:11:12 (offset 194)"), End: pos("testdata/vars.go:11:18 (offset 200)")},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.Filename, func(t *testing.T) {
			cont, err := os.ReadFile(c.Filename)
			if err != nil {
				t.Fatal(err)
			}
			fs := token.NewFileSet()
			astFile, err := parser.ParseFile(fs, c.Filename, cont, 0)
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig(fs, "refstest", []*ast.File{astFile})
			var allRefs []*posRef
			err = cfg.Refs(func(r *Ref) {
				allRefs = append(allRefs, &posRef{
					Def:   r.Def,
					Start: fs.Position(r.Start),
					End:   fs.Position(r.End),
				})
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
						t.Logf("    %d. %+v (offset start=%d end=%d)\n", i, r, r.Start.Offset, r.End.Offset)
					}
					t.Log("want", len(c.Want), "refs:")
					for i, r := range c.Want {
						t.Logf("    %d. %+v (offset start=%d end=%d)\n", i, r, r.Start.Offset, r.End.Offset)
					}
					t.FailNow()
				}
			}
		})
	}
}
