// +build !windows

package langserver

import (
	"github.com/sourcegraph/ctxvfs"
)

func bindLocalFs(fs *AtomicFS, mode ctxvfs.BindMode) {
	fs.Bind("/", ctxvfs.OS("/"), "/", mode)
}

func bindFs(fs *AtomicFS, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	fs.Bind("/", newfs, "/", mode)
}

func bindPath(path string, fs *AtomicFS, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	fs.Bind(path, newfs, "/", mode)
}
