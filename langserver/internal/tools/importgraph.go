// Original importgraph.Build contains the below copyright notice:
//
// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tools

import (
	"go/build"
	"sync"

	"golang.org/x/tools/go/buildutil"
	"golang.org/x/tools/refactor/importgraph"
)

// BuildReverseImportGraph is much like importgraph.Build, except:
// * it only returns the reverse graph
// * it does not return errors
//
// The code is adapted from the original function.
func BuildReverseImportGraph(ctxt *build.Context) importgraph.Graph {
	type importEdge struct {
		from, to string
	}

	ch := make(chan importEdge)

	go func() {
		sema := make(chan int, 20) // I/O concurrency limiting semaphore
		var wg sync.WaitGroup
		buildutil.ForEachPackage(ctxt, func(path string, err error) {
			if err != nil {
				return
			}

			wg.Add(1)
			go func() {
				defer wg.Done()

				sema <- 1
				// Even in error cases, Import usually returns a package.
				bp, _ := ctxt.Import(path, "", 0)
				<-sema

				// absolutize resolves an import path relative
				// to the current package bp.
				// The absolute form may contain "vendor".
				//
				// The vendoring feature slows down Build by 3×.
				// Here are timings from a 1400 package workspace:
				//    1100ms: current code (with vendor check)
				//     880ms: with a nonblocking cache around ctxt.IsDir
				//     840ms: nonblocking cache with duplicate suppression
				//     340ms: original code (no vendor check)
				// TODO(adonovan): optimize, somehow.
				memo := make(map[string]string)
				absolutize := func(path string) string {
					canon, ok := memo[path]
					if !ok {
						sema <- 1
						bp2, _ := ctxt.Import(path, bp.Dir, build.FindOnly)
						<-sema

						if bp2 != nil {
							canon = bp2.ImportPath
						} else {
							canon = path
						}
						memo[path] = canon
					}
					return canon
				}

				if bp != nil {
					for _, imp := range bp.Imports {
						ch <- importEdge{path, absolutize(imp)}
					}
					for _, imp := range bp.TestImports {
						ch <- importEdge{path, absolutize(imp)}
					}
					for _, imp := range bp.XTestImports {
						ch <- importEdge{path, absolutize(imp)}
					}
				}

			}()
		})
		wg.Wait()
		close(ch)
	}()

	reverse := make(importgraph.Graph)

	for e := range ch {
		if e.to == "C" {
			continue // "C" is fake
		}
		addEdge(reverse, e.to, e.from)
	}

	return reverse
}

func addEdge(g importgraph.Graph, from, to string) {
	edges := g[from]
	if edges == nil {
		edges = make(map[string]bool)
		g[from] = edges
	}
	edges[to] = true
}
