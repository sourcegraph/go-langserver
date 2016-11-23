package langserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/sourcegraph/ctxvfs"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/go-langserver/pkg/lspext"
	"github.com/sourcegraph/jsonrpc2"
)

func TestServer(t *testing.T) {
	tests := map[string]struct {
		rootPath                string
		fs                      map[string]string
		wantHover               map[string]string
		wantDefinition          map[string]string
		wantReferences          map[string][]string
		wantSymbols             map[string][]string
		wantWorkspaceSymbols    map[string][]string
		wantWorkspaceReferences []string
		mountFS                 map[string]map[string]string // mount dir -> map VFS
	}{
		"go basic": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": "package p; func A() { A() }",
				"b.go": "package p; func B() { A() }",
			},
			wantHover: map[string]string{
				"a.go:1:9":  "package p",
				"a.go:1:17": "func A()",
				"a.go:1:23": "func A()",
				"b.go:1:17": "func B()",
				"b.go:1:23": "func A()",
			},
			wantDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17",
				"a.go:1:23": "/src/test/pkg/a.go:1:17",
				"b.go:1:17": "/src/test/pkg/b.go:1:17",
				"b.go:1:23": "/src/test/pkg/a.go:1:17",
			},
			wantReferences: map[string][]string{
				"a.go:1:17": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
				"a.go:1:23": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
				"b.go:1:17": []string{"/src/test/pkg/b.go:1:17"},
				"b.go:1:23": []string{
					"/src/test/pkg/a.go:1:17",
					"/src/test/pkg/a.go:1:23",
					"/src/test/pkg/b.go:1:23",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
				"b.go": []string{"/src/test/pkg/b.go:function:pkg.B:1:17"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/a.go:function:pkg.A:1:17", "/src/test/pkg/b.go:function:pkg.B:1:17"},
				"A":           []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
				"B":           []string{"/src/test/pkg/b.go:function:pkg.B:1:17"},
				"is:exported": []string{"/src/test/pkg/a.go:function:pkg.A:1:17", "/src/test/pkg/b.go:function:pkg.B:1:17"},
				"dir:/":       []string{"/src/test/pkg/a.go:function:pkg.A:1:17", "/src/test/pkg/b.go:function:pkg.B:1:17"},
				"dir:/ A":     []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
				"dir:/ B":     []string{"/src/test/pkg/b.go:function:pkg.B:1:17"},
			},
			wantWorkspaceReferences: []string{},
		},
		"go detailed": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": "package p; type T struct { F string }",
			},
			wantHover: map[string]string{
				// "a.go:1:28": "(T).F string", // TODO(sqs): see golang/hover.go; this is the output we want
				"a.go:1:28": "struct field F string",
				"a.go:1:17": "type T struct",
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:class:pkg.T:1:17"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/a.go:class:pkg.T:1:17"},
				"T":           []string{"/src/test/pkg/a.go:class:pkg.T:1:17"},
				"F":           []string{}, // we don't return fields for now
				"is:exported": []string{"/src/test/pkg/a.go:class:pkg.T:1:17"},
			},
			wantWorkspaceReferences: []string{},
		},
		"exported defs unexported type": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": "package p; type t struct { F string }",
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:class:pkg.t:1:17"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"is:exported": []string{},
			},
			wantWorkspaceReferences: []string{},
		},
		"go xtest": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go":      "package p; var A int",
				"a_test.go": `package p_test; import "test/pkg"; var X = p.A`,
			},
			wantHover: map[string]string{
				"a.go:1:16":      "var A int",
				"a_test.go:1:40": "var X int",
				"a_test.go:1:46": "var A int",
			},
			wantWorkspaceReferences: []string{},
		},
		"go subdirectory in repo": {
			rootPath: "file:///src/test/pkg/d",
			fs: map[string]string{
				"a.go":    "package d; func A() { A() }",
				"d2/b.go": `package d2; import "test/pkg/d"; func B() { d.A(); B() }`,
			},
			wantHover: map[string]string{
				"a.go:1:17":    "func A()",
				"a.go:1:23":    "func A()",
				"d2/b.go:1:39": "func B()",
				"d2/b.go:1:47": "func A()",
				"d2/b.go:1:52": "func B()",
			},
			wantDefinition: map[string]string{
				"a.go:1:17":    "/src/test/pkg/d/a.go:1:17",
				"a.go:1:23":    "/src/test/pkg/d/a.go:1:17",
				"d2/b.go:1:39": "/src/test/pkg/d/d2/b.go:1:39",
				"d2/b.go:1:47": "/src/test/pkg/d/a.go:1:17",
				"d2/b.go:1:52": "/src/test/pkg/d/d2/b.go:1:39",
			},
			wantSymbols: map[string][]string{
				"a.go":    []string{"/src/test/pkg/d/a.go:function:d.A:1:17"},
				"d2/b.go": []string{"/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/d/a.go:function:d.A:1:17", "/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
				"is:exported": []string{"/src/test/pkg/d/a.go:function:d.A:1:17", "/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
				"dir:":        []string{"/src/test/pkg/d/a.go:function:d.A:1:17"},
				"dir:/":       []string{"/src/test/pkg/d/a.go:function:d.A:1:17"},
				"dir:.":       []string{"/src/test/pkg/d/a.go:function:d.A:1:17"},
				"dir:./":      []string{"/src/test/pkg/d/a.go:function:d.A:1:17"},
				"dir:/d2":     []string{"/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
				"dir:./d2":    []string{"/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
				"dir:d2/":     []string{"/src/test/pkg/d/d2/b.go:function:d2.B:1:39"},
			},
			wantWorkspaceReferences: []string{},
		},
		"go multiple packages in dir": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": "package p; func A() { A() }",
				"main.go": `// +build ignore

package main; import "test/pkg"; func B() { p.A(); B() }`,
			},
			wantHover: map[string]string{
				"a.go:1:17": "func A()",
				"a.go:1:23": "func A()",
				// Not parsing build-tag-ignored files:
				//
				// "main.go:3:39": "func B()", // func B()
				// "main.go:3:47": "func A()", // p.A()
				// "main.go:3:52": "func B()", // B()
			},
			wantDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17",
				"a.go:1:23": "/src/test/pkg/a.go:1:17",
				// Not parsing build-tag-ignored files:
				//
				// "main.go:3:39": "/src/test/pkg/main.go:3:39", // B() -> func B()
				// "main.go:3:47": "/src/test/pkg/a.go:1:17",    // p.A() -> a.go func A()
				// "main.go:3:52": "/src/test/pkg/main.go:3:39", // B() -> func B()
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
				"is:exported": []string{"/src/test/pkg/a.go:function:pkg.A:1:17"},
			},
			wantWorkspaceReferences: []string{},
		},
		"goroot": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package p; import "fmt"; var _ = fmt.Println; var x int`,
			},
			wantHover: map[string]string{
				"a.go:1:40": "func Println(a ...interface{}) (n int, err error)",
				"a.go:1:53": "type int int",
			},
			wantDefinition: map[string]string{
				"a.go:1:40": "/goroot/src/fmt/print.go:1:19",
				// "a.go:1:53": "/goroot/src/builtin/builtin.go:TODO:TODO", // TODO(sqs): support builtins
			},
			mountFS: map[string]map[string]string{
				"/goroot": {
					"src/fmt/print.go":       "package fmt; func Println(a ...interface{}) (n int, err error) { return }",
					"src/builtin/builtin.go": "package builtin; type int int",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{
					"/src/test/pkg/a.go:variable:pkg._:1:26",
					"/src/test/pkg/a.go:variable:pkg.x:1:47",
				},
				"": []string{},
			},
			wantWorkspaceSymbols: map[string][]string{
				"": []string{
					"/src/test/pkg/a.go:variable:pkg._:1:26",
					"/src/test/pkg/a.go:variable:pkg.x:1:47",
				},
				"is:exported": []string{},
			},
			wantWorkspaceReferences: []string{
				"/src/test/pkg/a.go:1:18 -> /goroot/src/fmt fmt/<none>",
				"/src/test/pkg/a.go:1:37 -> /goroot/src/fmt fmt/Println",
			},
		},
		"gopath": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a/a.go": `package a; func A() {}`,
				"b/b.go": `package b; import "test/pkg/a"; var _ = a.A`,
			},
			wantHover: map[string]string{
				"a/a.go:1:17": "func A()",
				// "b/b.go:1:20": "package", // TODO(sqs): make import paths hoverable
				"b/b.go:1:43": "func A()",
			},
			wantDefinition: map[string]string{
				"a/a.go:1:17": "/src/test/pkg/a/a.go:1:17",
				// "b/b.go:1:20": "/src/test/pkg/a", // TODO(sqs): make import paths hoverable
				"b/b.go:1:43": "/src/test/pkg/a/a.go:1:17",
			},
			wantReferences: map[string][]string{
				"a/a.go:1:17": []string{
					"/src/test/pkg/a/a.go:1:17",
					"/src/test/pkg/b/b.go:1:43",
				},
				"b/b.go:1:43": []string{ // calling "references" on call site should return same result as on decl
					"/src/test/pkg/a/a.go:1:17",
					"/src/test/pkg/b/b.go:1:43",
				},
			},
			wantSymbols: map[string][]string{
				"a/a.go": []string{"/src/test/pkg/a/a.go:function:a.A:1:17"},
				"b/b.go": []string{"/src/test/pkg/b/b.go:variable:b._:1:33"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/a/a.go:function:a.A:1:17", "/src/test/pkg/b/b.go:variable:b._:1:33"},
				"is:exported": []string{"/src/test/pkg/a/a.go:function:a.A:1:17"},
			},
			wantWorkspaceReferences: []string{},
		},
		"go vendored dep": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package a; import "github.com/v/vendored"; var _ = vendored.V`,
				"vendor/github.com/v/vendored/v.go": "package vendored; func V() {}",
			},
			wantHover: map[string]string{
				"a.go:1:61": "func V()",
			},
			wantDefinition: map[string]string{
				"a.go:1:61": "/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24",
			},
			wantReferences: map[string][]string{
				"vendor/github.com/v/vendored/v.go:1:24": []string{
					"/src/test/pkg/vendor/github.com/v/vendored/v.go:1:24",
					"/src/test/pkg/a.go:1:61",
				},
			},
			wantSymbols: map[string][]string{
				"a.go": []string{"/src/test/pkg/a.go:variable:pkg._:1:44"},
				"vendor/github.com/v/vendored/v.go": []string{"/src/test/pkg/vendor/github.com/v/vendored/v.go:function:vendored.V:1:24"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/a.go:variable:pkg._:1:44", "/src/test/pkg/vendor/github.com/v/vendored/v.go:function:vendored.V:1:24"},
				"is:exported": []string{},
			},
			wantWorkspaceReferences: []string{},
		},
		"go vendor symbols with same name": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"z.go": `package pkg; func x() bool { return true }`,
				"vendor/github.com/a/pkg2/x.go": `package pkg2; func x() bool { return true }`,
				"vendor/github.com/x/pkg3/x.go": `package pkg3; func x() bool { return true }`,
			},
			wantSymbols: map[string][]string{
				"z.go": []string{"/src/test/pkg/z.go:function:pkg.x:1:19"},
				"vendor/github.com/a/pkg2/x.go": []string{"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:pkg2.x:1:20"},
				"vendor/github.com/x/pkg3/x.go": []string{"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:pkg3.x:1:20"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"": []string{
					"/src/test/pkg/z.go:function:pkg.x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:pkg2.x:1:20",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:pkg3.x:1:20",
				},
				"x": []string{
					"/src/test/pkg/z.go:function:pkg.x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:pkg2.x:1:20",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:pkg3.x:1:20",
				},
				"pkg2.x": []string{
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:pkg2.x:1:20",
					"/src/test/pkg/z.go:function:pkg.x:1:19",
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:pkg3.x:1:20",
				},
				"pkg3.x": []string{
					"/src/test/pkg/vendor/github.com/x/pkg3/x.go:function:pkg3.x:1:20",
					"/src/test/pkg/z.go:function:pkg.x:1:19",
					"/src/test/pkg/vendor/github.com/a/pkg2/x.go:function:pkg2.x:1:20",
				},
				"is:exported": []string{},
			},
			wantWorkspaceReferences: []string{},
		},
		"go external dep": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package a; import "github.com/d/dep"; var _ = dep.D; var _ = dep.D`,
			},
			wantHover: map[string]string{
				"a.go:1:51": "func D()",
			},
			wantDefinition: map[string]string{
				"a.go:1:51": "/src/github.com/d/dep/d.go:1:19",
			},
			wantReferences: map[string][]string{
				"a.go:1:51": []string{
					"/src/test/pkg/a.go:1:51",
					"/src/test/pkg/a.go:1:66",
					// Do not include "refs" from the dependency
					// package itself; only return results in the
					// workspace.
				},
			},
			wantWorkspaceReferences: []string{
				"/src/test/pkg/a.go:1:18 -> /src/github.com/d/dep dep/<none>",
				"/src/test/pkg/a.go:1:50 -> /src/github.com/d/dep dep/D",
				"/src/test/pkg/a.go:1:65 -> /src/github.com/d/dep dep/D",
			},
			mountFS: map[string]map[string]string{
				"/src/github.com/d/dep": {
					"d.go": "package dep; func D() {}; var _ = D",
				},
			},
		},
		"external dep with vendor": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package p; import "github.com/d/dep"; var _ = dep.D().F`,
			},
			wantDefinition: map[string]string{
				"a.go:1:55": "/src/github.com/d/dep/vendor/vendp/vp.go:1:32",
			},
			wantWorkspaceReferences: []string{
				"/src/test/pkg/a.go:1:18 -> /src/github.com/d/dep dep/<none>",
				"/src/test/pkg/a.go:1:54 -> /src/github.com/d/dep/vendor/vendp F/V",
				"/src/test/pkg/a.go:1:50 -> /src/github.com/d/dep dep/D",
			},
			mountFS: map[string]map[string]string{
				"/src/github.com/d/dep": map[string]string{
					"d.go":               `package dep; import "vendp"; func D() (v vendp.V) { return }`,
					"vendor/vendp/vp.go": "package vendp; type V struct { F int }",
				},
			},
		},
		"go external dep at subtree": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package a; import "github.com/d/dep/subp"; var _ = subp.D`,
			},
			wantHover: map[string]string{
				"a.go:1:57": "func D()",
			},
			wantDefinition: map[string]string{
				"a.go:1:57": "/src/github.com/d/dep/subp/d.go:1:20",
			},
			wantWorkspaceReferences: []string{
				"/src/test/pkg/a.go:1:18 -> /src/github.com/d/dep/subp subp/<none>",
				"/src/test/pkg/a.go:1:56 -> /src/github.com/d/dep/subp subp/D",
			},
			mountFS: map[string]map[string]string{
				"/src/github.com/d/dep": {
					"subp/d.go": "package subp; func D() {}",
				},
			},
		},
		"go nested external dep": { // a depends on dep1, dep1 depends on dep2
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `package a; import "github.com/d/dep1"; var _ = dep1.D1().D2`,
			},
			wantHover: map[string]string{
				"a.go:1:53": "func D1() D2",
				"a.go:1:59": "struct field D2 int",
			},
			wantDefinition: map[string]string{
				"a.go:1:53": "/src/github.com/d/dep1/d1.go:1:48", // func D1
				"a.go:1:58": "/src/github.com/d/dep2/d2.go:1:32", // field D2
			},
			wantWorkspaceReferences: []string{
				"/src/test/pkg/a.go:1:18 -> /src/github.com/d/dep1 dep1/<none>",
				"/src/test/pkg/a.go:1:57 -> /src/github.com/d/dep2 D2/D2",
				"/src/test/pkg/a.go:1:52 -> /src/github.com/d/dep1 dep1/D1",
			},
			mountFS: map[string]map[string]string{
				"/src/github.com/d/dep1": {
					"d1.go": `package dep1; import "github.com/d/dep2"; func D1() dep2.D2 { return dep2.D2{} }`,
				},
				"/src/github.com/d/dep2": {
					"d2.go": "package dep2; type D2 struct { D2 int }",
				},
			},
		},
		"go symbols": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"abc.go": `package a

type XYZ struct {}

func (x XYZ) ABC() {}
`,
				"bcd.go": `package a

type YZA struct {}

func (y YZA) BCD() {}
`,
				"xyz.go": `package a

func yza() {}
`,
			},
			wantSymbols: map[string][]string{
				"abc.go": []string{"/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/abc.go:class:pkg.XYZ:3:6"},
				"bcd.go": []string{"/src/test/pkg/bcd.go:method:YZA.BCD:5:14", "/src/test/pkg/bcd.go:class:pkg.YZA:3:6"},
				"xyz.go": []string{"/src/test/pkg/xyz.go:function:pkg.yza:3:6"},
			},
			wantWorkspaceSymbols: map[string][]string{
				"":            []string{"/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14", "/src/test/pkg/abc.go:class:pkg.XYZ:3:6", "/src/test/pkg/bcd.go:class:pkg.YZA:3:6", "/src/test/pkg/xyz.go:function:pkg.yza:3:6"},
				"xyz":         []string{"/src/test/pkg/abc.go:class:pkg.XYZ:3:6", "/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/xyz.go:function:pkg.yza:3:6"},
				"yza":         []string{"/src/test/pkg/bcd.go:class:pkg.YZA:3:6", "/src/test/pkg/xyz.go:function:pkg.yza:3:6", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14"},
				"abc":         []string{"/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/abc.go:class:pkg.XYZ:3:6"},
				"bcd":         []string{"/src/test/pkg/bcd.go:method:YZA.BCD:5:14", "/src/test/pkg/bcd.go:class:pkg.YZA:3:6"},
				"is:exported": []string{"/src/test/pkg/abc.go:method:XYZ.ABC:5:14", "/src/test/pkg/bcd.go:method:YZA.BCD:5:14", "/src/test/pkg/abc.go:class:pkg.XYZ:3:6", "/src/test/pkg/bcd.go:class:pkg.YZA:3:6"},
			},
			wantWorkspaceReferences: []string{},
		},
		"go hover docs": {
			rootPath: "file:///src/test/pkg",
			fs: map[string]string{
				"a.go": `// Copyright 2015 someone.
// Copyrights often span multiple lines.

// Some additional non-package docs.

// Package p is a package with lots of great things.
package p

import "github.com/a/pkg2"

// logit is pkg2.X
var logit = pkg2.X

// T is a struct.
type T struct {
	// F is a string field.
	F string

	// H is a header.
	H pkg2.Header
}

// Foo is the best string.
var Foo string
`,
				"vendor/github.com/a/pkg2/x.go": `// Package pkg2 shows dependencies.
//
// How to
//
// 	Example Code!
//
package pkg2

// X does the unknown.
func X() {
	panic("zomg")
}

// Header is like an HTTP header, only better.
type Header struct {
	// F is a string, too.
	F string
}
`,
			},
			wantHover: map[string]string{
				"a.go:7:9": "package p; Package p is a package with lots of great things. \n\n",
				//"a.go:9:9": "", TODO: handle hovering on import statements (ast.BasicLit)
				"a.go:12:5":  "var logit func(); logit is pkg2.X \n\n",
				"a.go:12:13": "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:12:18": "func X(); X does the unknown. \n\n",
				"a.go:15:6":  "type T struct; T is a struct. \n\n",
				"a.go:17:2":  "struct field F string; F is a string field. \n\n",
				"a.go:20:2":  "struct field H test/pkg/vendor/github.com/a/pkg2.Header; H is a header. \n\n",
				"a.go:20:4":  "package pkg2 (\"test/pkg/vendor/github.com/a/pkg2\"); Package pkg2 shows dependencies. \n\nHow to \n\n```\nExample Code!\n\n```\n",
				"a.go:24:5":  "var Foo string; Foo is the best string. \n\n",
			},
		},
	}
	for label, test := range tests {
		t.Run(label, func(t *testing.T) {
			h := &LangHandler{HandlerShared: &HandlerShared{}}

			addr, done := startServer(t, jsonrpc2.HandlerWithError(h.handle))
			defer done()
			conn := dialServer(t, addr)
			defer func() {
				if err := conn.Close(); err != nil {
					t.Fatal("conn.Close:", err)
				}
			}()

			rootFSPath := strings.TrimPrefix(test.rootPath, "file://")

			// Prepare the connection.
			ctx := context.Background()
			if err := conn.Call(ctx, "initialize", InitializeParams{
				InitializeParams:     lsp.InitializeParams{RootPath: test.rootPath},
				NoOSFileSystemAccess: true,
				RootImportPath:       strings.TrimPrefix(rootFSPath, "/src/"),
				BuildContext: &InitializeBuildContextParams{
					GOOS:     "linux",
					GOARCH:   "amd64",
					GOPATH:   "/",
					GOROOT:   "/goroot",
					Compiler: runtime.Compiler,
				},
			}, nil); err != nil {
				t.Fatal("initialize:", err)
			}

			h.Mu.Lock()
			h.FS.Bind(rootFSPath, mapFS(test.fs), "/", ctxvfs.BindReplace)
			for mountDir, fs := range test.mountFS {
				h.FS.Bind(mountDir, mapFS(fs), "/", ctxvfs.BindAfter)
			}
			h.Mu.Unlock()

			lspTests(t, ctx, conn, rootFSPath, test.wantHover, test.wantDefinition, test.wantReferences, test.wantSymbols, test.wantWorkspaceSymbols, test.wantWorkspaceReferences)
		})
	}
}

func startServer(t testing.TB, h jsonrpc2.Handler) (addr string, done func()) {
	bindAddr := ":0"
	if os.Getenv("CI") != "" {
		// CircleCI has issues with IPv6 (e.g., "dial tcp [::]:39984:
		// connect: network is unreachable").
		bindAddr = "127.0.0.1:0"
	}
	l, err := net.Listen("tcp", bindAddr)
	if err != nil {
		t.Fatal("Listen:", err)
	}
	go func() {
		if err := jsonrpc2.Serve(context.Background(), l, h); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			t.Fatal("jsonrpc2.Serve:", err)
		}
	}()
	return l.Addr().String(), func() {
		if err := l.Close(); err != nil {
			t.Fatal("close listener:", err)
		}
	}
}

func dialServer(t testing.TB, addr string) *jsonrpc2.Conn {
	conn, err := (&net.Dialer{}).Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return jsonrpc2.NewConn(context.Background(), conn, jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (interface{}, error) {
		// no-op
		return nil, nil
	}))
}

// lspTests runs all test suites for LSP functionality.
func lspTests(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, wantHover, wantDefinition map[string]string, wantReferences, wantSymbols, wantWorkspaceSymbols map[string][]string, wantWorkspaceReferences []string) {
	for pos, want := range wantHover {
		tbRun(t, fmt.Sprintf("hover-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			hoverTest(t, ctx, c, rootPath, pos, want)
		})
	}

	for pos, want := range wantDefinition {
		tbRun(t, fmt.Sprintf("definition-%s", strings.Replace(pos, "/", "-", -1)), func(t testing.TB) {
			definitionTest(t, ctx, c, rootPath, pos, want)
		})
	}

	for pos, want := range wantReferences {
		tbRun(t, fmt.Sprintf("references-%s", pos), func(t testing.TB) {
			referencesTest(t, ctx, c, rootPath, pos, want)
		})
	}

	for file, want := range wantSymbols {
		tbRun(t, fmt.Sprintf("symbols-%s", file), func(t testing.TB) {
			symbolsTest(t, ctx, c, rootPath, file, want)
		})
	}

	for query, want := range wantWorkspaceSymbols {
		tbRun(t, fmt.Sprintf("workspaceSymbols(q=%q)", query), func(t testing.TB) {
			workspaceSymbolsTest(t, ctx, c, rootPath, query, want)
		})
	}

	if wantWorkspaceReferences != nil {
		tbRun(t, "workspaceReferences", func(t testing.TB) {
			workspaceReferencesTest(t, ctx, c, rootPath, wantWorkspaceReferences)
		})
	}
}

// tbRun calls (testing.T).Run or (testing.B).Run.
func tbRun(t testing.TB, name string, f func(testing.TB)) bool {
	switch tb := t.(type) {
	case *testing.B:
		return tb.Run(name, func(b *testing.B) { f(b) })
	case *testing.T:
		return tb.Run(name, func(t *testing.T) { f(t) })
	default:
		panic(fmt.Sprintf("unexpected %T, want *testing.B or *testing.T", tb))
	}
}

func hoverTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	hover, err := callHover(ctx, c, "file://"+path.Join(rootPath, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	if hover != want {
		t.Fatalf("got %q, want %q", hover, want)
	}
}

func definitionTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, pos, want string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := callDefinition(ctx, c, "file://"+path.Join(rootPath, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	definition = strings.TrimPrefix(definition, "file://")
	if definition != want {
		t.Errorf("got %q, want %q", definition, want)
	}
}

func referencesTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, pos string, want []string) {
	file, line, char, err := parsePos(pos)
	if err != nil {
		t.Fatal(err)
	}
	references, err := callReferences(ctx, c, "file://"+path.Join(rootPath, file), line, char)
	if err != nil {
		t.Fatal(err)
	}
	for i := range references {
		references[i] = strings.TrimPrefix(references[i], "file://")
	}
	sort.Strings(references)
	sort.Strings(want)
	if !reflect.DeepEqual(references, want) {
		t.Errorf("got %q, want %q", references, want)
	}
}

func symbolsTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, file string, want []string) {
	symbols, err := callSymbols(ctx, c, "file://"+path.Join(rootPath, file))
	if err != nil {
		t.Fatal(err)
	}
	for i := range symbols {
		symbols[i] = strings.TrimPrefix(symbols[i], "file://")
	}
	if !reflect.DeepEqual(symbols, want) {
		t.Errorf("got %q, want %q", symbols, want)
	}
}

func workspaceSymbolsTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, query string, want []string) {
	symbols, err := callWorkspaceSymbols(ctx, c, query)
	if err != nil {
		t.Fatal(err)
	}
	for i := range symbols {
		symbols[i] = strings.TrimPrefix(symbols[i], "file://")
	}
	if !reflect.DeepEqual(symbols, want) {
		t.Errorf("got %#v, want %q", symbols, want)
	}
}

func workspaceReferencesTest(t testing.TB, ctx context.Context, c *jsonrpc2.Conn, rootPath string, want []string) {
	references, err := callWorkspaceReferences(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(references, want) {
		t.Errorf("got %#v, want %q", references, want)
	}
}

func parsePos(s string) (file string, line, char int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		err = fmt.Errorf("invalid pos %q (%d parts)", s, len(parts))
		return
	}
	file = parts[0]
	line, err = strconv.Atoi(parts[1])
	if err != nil {
		err = fmt.Errorf("invalid line in %q: %s", s, err)
		return
	}
	char, err = strconv.Atoi(parts[2])
	if err != nil {
		err = fmt.Errorf("invalid char in %q: %s", s, err)
		return
	}
	return file, line - 1, char - 1, nil // LSP is 0-indexed
}

func callHover(ctx context.Context, c *jsonrpc2.Conn, uri string, line, char int) (string, error) {
	var res struct {
		Contents markedStrings `json:"contents"`
		lsp.Hover
	}
	err := c.Call(ctx, "textDocument/hover", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, ms := range res.Contents {
		if i != 0 {
			str += "; "
		}
		str += ms.Value
	}
	return str, nil
}

func callDefinition(ctx context.Context, c *jsonrpc2.Conn, uri string, line, char int) (string, error) {
	var res locations
	err := c.Call(ctx, "textDocument/definition", lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: char},
	}, &res)
	if err != nil {
		return "", err
	}
	var str string
	for i, loc := range res {
		if loc.URI == "" {
			continue
		}
		if i != 0 {
			str += ", "
		}
		str += fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}
	return str, nil
}

func callReferences(ctx context.Context, c *jsonrpc2.Conn, uri string, line, char int) ([]string, error) {
	var res locations
	err := c.Call(ctx, "textDocument/references", lsp.ReferenceParams{
		Context: lsp.ReferenceContext{IncludeDeclaration: true},
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: line, Character: char},
		},
	}, &res)
	if err != nil {
		return nil, err
	}
	str := make([]string, len(res))
	for i, loc := range res {
		str[i] = fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}
	return str, nil
}

var symbolKindName = map[lsp.SymbolKind]string{
	lsp.SKFile:        "file",
	lsp.SKModule:      "module",
	lsp.SKNamespace:   "namespace",
	lsp.SKPackage:     "package",
	lsp.SKClass:       "class",
	lsp.SKMethod:      "method",
	lsp.SKProperty:    "property",
	lsp.SKField:       "field",
	lsp.SKConstructor: "constructor",
	lsp.SKEnum:        "enum",
	lsp.SKInterface:   "interface",
	lsp.SKFunction:    "function",
	lsp.SKVariable:    "variable",
	lsp.SKConstant:    "constant",
	lsp.SKString:      "string",
	lsp.SKNumber:      "number",
	lsp.SKBoolean:     "boolean",
	lsp.SKArray:       "array",
}

func callSymbols(ctx context.Context, c *jsonrpc2.Conn, uri string) ([]string, error) {
	var symbols []lsp.SymbolInformation
	err := c.Call(ctx, "textDocument/documentSymbol", lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	}, &symbols)
	if err != nil {
		return nil, err
	}
	syms := make([]string, len(symbols))
	for i, s := range symbols {
		syms[i] = fmt.Sprintf("%s:%s:%s.%s:%d:%d", s.Location.URI, symbolKindName[s.Kind], s.ContainerName, s.Name, s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return syms, nil
}

func callWorkspaceSymbols(ctx context.Context, c *jsonrpc2.Conn, query string) ([]string, error) {
	var symbols []lsp.SymbolInformation
	err := c.Call(ctx, "workspace/symbol", lsp.WorkspaceSymbolParams{Query: query}, &symbols)
	if err != nil {
		return nil, err
	}
	syms := make([]string, len(symbols))
	for i, s := range symbols {
		syms[i] = fmt.Sprintf("%s:%s:%s.%s:%d:%d", s.Location.URI, symbolKindName[s.Kind], s.ContainerName, s.Name, s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return syms, nil
}

func callWorkspaceReferences(ctx context.Context, c *jsonrpc2.Conn) ([]string, error) {
	var references []lspext.ReferenceInformation
	err := c.Call(ctx, "workspace/reference", lspext.WorkspaceReferenceParams{}, &references)
	if err != nil {
		return nil, err
	}
	refs := make([]string, len(references))
	for i, r := range references {
		start := r.Location.Range.Start
		if r.ContainerName == "" {
			r.ContainerName = "<none>"
		}
		if r.Name == "" {
			r.Name = "<none>"
		}
		path := strings.Join([]string{r.ContainerName, r.Name}, "/")
		locationURI := strings.TrimPrefix(r.Location.URI, "file://")
		uri := strings.TrimPrefix(r.URI, "file://")
		refs[i] = fmt.Sprintf("%s:%d:%d -> %s %s", locationURI, start.Line+1, start.Character+1, uri, path)
	}
	return refs, nil
}

type markedStrings []lsp.MarkedString

func (v *markedStrings) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '[' {
		var ms []markedString
		if err := json.Unmarshal(data, &ms); err != nil {
			return err
		}
		for _, ms := range ms {
			*v = append(*v, lsp.MarkedString(ms))
		}
		return nil
	}
	*v = []lsp.MarkedString{{}}
	return json.Unmarshal(data, &(*v)[0])
}

type markedString lsp.MarkedString

func (v *markedString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '{' {
		return json.Unmarshal(data, (*lsp.MarkedString)(v))
	}

	// String
	*v = markedString{}
	return json.Unmarshal(data, &v.Value)
}

type locations []lsp.Location

func (v *locations) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid empty JSON")
	}
	if data[0] == '[' {
		return json.Unmarshal(data, (*[]lsp.Location)(v))
	}
	*v = []lsp.Location{{}}
	return json.Unmarshal(data, &(*v)[0])
}

// testRequest is a simplified version of jsonrpc2.Request for easier
// test expectation definition and checking of the fields that matter.
type testRequest struct {
	Method string
	Params interface{}
}

func (r testRequest) String() string {
	b, err := json.Marshal(r.Params)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s(%s)", r.Method, b)
}

func testRequestEqual(a, b testRequest) bool {
	if a.Method != b.Method {
		return false
	}

	// We want to see if a and b have identical canonical JSON
	// representations. They are NOT identical Go structures, since
	// one comes from the wire (as raw JSON) and one is an interface{}
	// of a concrete struct/slice type provided as a test expectation.
	ajson, err := json.Marshal(a.Params)
	if err != nil {
		panic(err)
	}
	bjson, err := json.Marshal(b.Params)
	if err != nil {
		panic(err)
	}
	var a2, b2 interface{}
	if err := json.Unmarshal(ajson, &a2); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(bjson, &b2); err != nil {
		panic(err)
	}
	return reflect.DeepEqual(a2, b2)
}

func testRequestsEqual(as, bs []testRequest) bool {
	if len(as) != len(bs) {
		return false
	}
	for i, a := range as {
		if !testRequestEqual(a, bs[i]) {
			return false
		}
	}
	return true
}

type testRequests []testRequest

func (v testRequests) Len() int      { return len(v) }
func (v testRequests) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v testRequests) Less(i, j int) bool {
	ii, err := json.Marshal(v[i])
	if err != nil {
		panic(err)
	}
	jj, err := json.Marshal(v[j])
	if err != nil {
		panic(err)
	}
	return string(ii) < string(jj)
}

// mapFS lets us easily instantiate a VFS with a map[string]string
// (which is less noisy than map[string][]byte in test fixtures).
func mapFS(m map[string]string) ctxvfs.FileSystem {
	m2 := make(map[string][]byte, len(m))
	for k, v := range m {
		m2[k] = []byte(v)
	}
	return ctxvfs.Map(m2)
}
