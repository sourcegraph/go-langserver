// +build !windows

package langserver

import (
	"github.com/sourcegraph/ctxvfs"
)

func bindLocalFs(ns ctxvfs.NameSpace, mode ctxvfs.BindMode) {
	ns.Bind("/", ctxvfs.OS("/"), "/", mode)
}

func bindFs(ns ctxvfs.NameSpace, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	ns.Bind("/", newfs, "/", mode)
}

func bindPath(path string, ns ctxvfs.NameSpace, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	ns.Bind(path, newfs, "/", mode)
}
