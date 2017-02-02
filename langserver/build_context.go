package langserver

import (
	"fmt"
	"go/build"
	"io"
	"os"
	"path"
	"strings"

	"golang.org/x/net/context"

	"golang.org/x/tools/go/buildutil"
)

// BuildContext creates a build.Context which uses the overlay FS and the InitializeParams.BuildContext overrides.
func (h *LangHandler) BuildContext(ctx context.Context) *build.Context {
	var bctx *build.Context
	if override := h.init.BuildContext; override != nil {
		bctx = &build.Context{
			GOOS:        override.GOOS,
			GOARCH:      override.GOARCH,
			GOPATH:      override.GOPATH,
			GOROOT:      override.GOROOT,
			CgoEnabled:  override.CgoEnabled,
			UseAllFiles: override.UseAllFiles,
			Compiler:    override.Compiler,
			BuildTags:   override.BuildTags,

			// Enable analysis of all go version build tags that
			// our compiler should understand.
			ReleaseTags: build.Default.ReleaseTags,
		}
	} else {
		// make a copy since we will mutate it
		copy := build.Default
		bctx = &copy
	}

	h.Mu.Lock()
	fs := h.FS
	h.Mu.Unlock()

	// HACK: in the all Context's methods below we are trying to convert path to virtual one (/foo/bar/..)
	// because some code may pass OS-specific arguments.
	// See golang.org/x/tools/go/buildutil/allpackages.go which uses `filepath` for example

	bctx.OpenFile = func(path string) (io.ReadCloser, error) {
		return fs.Open(ctx, virtualPath(path))
	}
	bctx.IsDir = func(path string) bool {
		fi, err := fs.Stat(ctx, virtualPath(path))
		return err == nil && fi.Mode().IsDir()
	}
	bctx.HasSubdir = func(root, dir string) (rel string, ok bool) {
		root = virtualPath(root)
		dir = virtualPath(dir)
		if !bctx.IsDir(dir) {
			return "", false
		}
		if !PathHasPrefix(dir, root) {
			return "", false
		}
		return PathTrimPrefix(dir, root), true
	}
	bctx.ReadDir = func(path string) ([]os.FileInfo, error) {
		return fs.ReadDir(ctx, virtualPath(path))
	}
	bctx.IsAbsPath = func(path string) bool {
		return isAbs(path)
	}
	bctx.JoinPath = path.Join
	return bctx
}

// ContainingPackage returns the package that contains the given
// filename. It is like buildutil.ContainingPackage, except that:
//
// * it returns the whole package (i.e., it doesn't use build.FindOnly)
// * it does not perform FS calls that are unnecessary for us (such
//   as searching the GOROOT; this is only called on the main
//   workspace's code, not its deps).
// * if the file is in the xtest package (package p_test not package p),
//   it returns build.Package only representing that xtest package
func ContainingPackage(bctx *build.Context, filename string) (*build.Package, error) {
	gopaths := buildutil.SplitPathList(bctx, bctx.GOPATH) // list will be empty with no GOPATH
	for _, gopath := range gopaths {
		if !buildutil.IsAbsPath(bctx, gopath) {
			return nil, fmt.Errorf("build context GOPATH must be an absolute path (GOPATH=%q)", gopath)
		}
	}

	pkgDir := filename
	if !bctx.IsDir(filename) {
		pkgDir = path.Dir(filename)
	}
	var srcDir string
	if PathHasPrefix(filename, bctx.GOROOT) {
		srcDir = bctx.GOROOT // if workspace is Go stdlib
	} else {
		srcDir = "" // with no GOPATH, only stdlib will work
		for _, gopath := range gopaths {
			if PathHasPrefix(pkgDir, gopath) {
				srcDir = gopath
				break
			}
		}
	}
	srcDir = path.Join(srcDir, "src")
	importPath := PathTrimPrefix(pkgDir, srcDir)
	var xtest bool
	pkg, err := bctx.Import(importPath, pkgDir, 0)
	if pkg != nil {
		base := path.Base(filename)
		for _, f := range pkg.XTestGoFiles {
			if f == base {
				xtest = true
				break
			}
		}
	}

	// If the filename we want refers to a file in an xtest package
	// (package p_test not package p), then munge the package so that
	// it only refers to that xtest package.
	if pkg != nil && xtest && !strings.HasSuffix(pkg.Name, "_test") {
		pkg.Name += "_test"
		pkg.GoFiles = nil
		pkg.CgoFiles = nil
		pkg.TestGoFiles = nil
	}

	return pkg, err
}
