package tools

import (
	"go/build"
	"path"
	"sort"
	"sync"

	"golang.org/x/tools/go/buildutil"

	"github.com/sourcegraph/go-langserver/langserver/util"
)

// ListPkgsUnderDir is buildutil.ExpandPattern(ctxt, []string{dir +
// "/..."}). The implementation is modified from the upstream
// buildutil.ExpandPattern so we can be much faster. buildutil.ExpandPattern
// looks at all directories under GOPATH if there is a `...` pattern. This
// instead only explores the directories under dir. In future
// buildutil.ExpandPattern may be more performant (there are TODOs for it).
func ListPkgsUnderDir(ctxt *build.Context, dir string) []string {
	ch := make(chan string)
	dir = path.Clean(dir)

	var (
		wg          sync.WaitGroup
		dirInGOPATH bool
	)

	for _, root := range ctxt.SrcDirs() {
		root = path.Clean(root)

		if util.PathHasPrefix(root, dir) {
			// If we are a child of dir, we can just start at the
			// root. A concrete example of this happening is when
			// root=/goroot/src and dir=/goroot
			dir = root
		}

		if !util.PathHasPrefix(dir, root) {
			continue
		}

		wg.Add(1)
		go func() {
			allPackages(ctxt, root, dir, ch)
			wg.Done()
		}()
		dirInGOPATH = true
	}

	if !dirInGOPATH {
		root := path.Dir(dir)
		wg.Add(1)
		go func() {
			allPackages(ctxt, root, dir, ch)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var pkgs []string
	for p := range ch {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs
}

// We use a process-wide counting semaphore to limit
// the number of parallel calls to ReadDir.
var ioLimit = make(chan bool, 20)

// allPackages is from tools/go/buildutil. We don't use the exported method
// since it doesn't allow searching from a directory. We need from a specific
// directory for performance on large GOPATHs.
func allPackages(ctxt *build.Context, root, start string, ch chan<- string) {

	var wg sync.WaitGroup

	var walkDir func(dir string)
	walkDir = func(dir string) {
		// Avoid .foo, _foo, and testdata directory trees.
		base := path.Base(dir)
		if base == "" || base[0] == '.' || base[0] == '_' || base == "testdata" {
			return
		}

		pkg := util.PathTrimPrefix(dir, root)

		// Prune search if we encounter any of these import paths.
		switch pkg {
		case "builtin":
			return
		}

		if pkg != "" {
			ch <- pkg
		}

		ioLimit <- true
		files, _ := buildutil.ReadDir(ctxt, dir)
		<-ioLimit
		for _, fi := range files {
			fi := fi
			if fi.IsDir() {
				wg.Add(1)
				go func() {
					walkDir(buildutil.JoinPath(ctxt, dir, fi.Name()))
					wg.Done()
				}()
			}
		}
	}

	walkDir(start)
	wg.Wait()
}
