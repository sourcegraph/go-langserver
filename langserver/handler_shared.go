package langserver

import (
	"context"
	"go/build"
	"path/filepath"
	"sync"

	"github.com/sourcegraph/ctxvfs"

	"github.com/sourcegraph/go-langserver/langserver/util"
)

// HandlerShared contains data structures that a build server and its
// wrapped lang server may share in memory.
type HandlerShared struct {
	Mu     sync.Mutex // guards all fields
	Shared bool       // true if this struct is shared with a build server
	FS     *AtomicFS  // full filesystem (mounts both deps and overlay)

	// FindPackage if non-nil is used by our typechecker. See
	// loader.Config.FindPackage. We use this in production to lazily
	// fetch dependencies + cache lookups.
	FindPackage FindPackageFunc

	overlay *overlay // files to overlay
}

// FindPackageFunc matches the signature of loader.Config.FindPackage, except
// also takes a context.Context.
type FindPackageFunc func(ctx context.Context, bctx *build.Context, importPath, fromDir string, mode build.ImportMode) (*build.Package, error)

func defaultFindPackageFunc(ctx context.Context, bctx *build.Context, importPath, fromDir string, mode build.ImportMode) (*build.Package, error) {
	return bctx.Import(importPath, fromDir, mode)
}

// getFindPackageFunc is a helper which returns h.FindPackage if non-nil, otherwise defaultFindPackageFunc
func (h *HandlerShared) getFindPackageFunc(rootPath string) FindPackageFunc {
	return func(ctx context.Context, bctx *build.Context, importPath, fromDir string, mode build.ImportMode) (*build.Package, error) {
		var (
			res        *build.Package
			err        error
			importFunc FindPackageFunc
		)

		if h.FindPackage != nil {
			importFunc = h.FindPackage
		} else {
			importFunc = defaultFindPackageFunc
		}

		var srcDir string
		if util.PathHasPrefix(rootPath, bctx.GOROOT) {
			srcDir = bctx.GOROOT // if workspace is Go stdlib
		} else {
			gopaths := filepath.SplitList(bctx.GOPATH)
			for _, gopath := range gopaths {
				if util.PathHasPrefix(rootPath, gopath) {
					srcDir = gopath
					break
				}
			}
		}

		res, err = importFunc(ctx, bctx, importPath, fromDir, mode)
		if err != nil && srcDir == "" {
			// Workspace is out of GOPATH, we have 3 fallback dirs:
			// 1. local package;
			// 2. project level vendored package;
			// 3. nested vendored package.
			// Packages in go.mod file but not in vendor dir are not supported yet. :(
			fallBackDirs := make([]string, 0, 3)

			// Local imports always have same prefix -- the current dir's name.
			if util.PathHasPrefix(importPath, filepath.Base(rootPath)) {
				fallBackDirs = append(fallBackDirs, filepath.Join(filepath.Dir(rootPath), importPath))
			}
			// Vendored package.
			fallBackDirs = append(fallBackDirs, filepath.Join(rootPath, "vendor", importPath))
			// Nested vendored packages.
			if fromDir != rootPath && fromDir != "" {
				fallBackDirs = append(fallBackDirs, filepath.Join(fromDir, "vendor", importPath))
			}

			// In case of import error, use ImportDir instead.
			// We must set ImportPath manually.
			for _, importDir := range fallBackDirs {
				res, err = bctx.ImportDir(importDir, mode)
				if res != nil {
					res.ImportPath = importPath
				}
				if err == nil {
					break
				}
				if _, ok := err.(*build.NoGoError); ok {
					break
				}
				if _, ok := err.(*build.MultiplePackageError); ok {
					break
				}
			}
		}

		return res, err
	}
}

func (h *HandlerShared) Reset(useOSFS bool) error {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.overlay = newOverlay()
	h.FS = NewAtomicFS()

	if useOSFS {
		// The overlay FS takes precedence, but we fall back to the OS
		// file system.
		h.FS.Bind("/", ctxvfs.OS("/"), "/", ctxvfs.BindAfter)
	}
	h.FS.Bind("/", h.overlay.FS(), "/", ctxvfs.BindBefore)
	return nil
}
