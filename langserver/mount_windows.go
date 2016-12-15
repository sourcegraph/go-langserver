// +build windows

package langserver

import (
	"path/filepath"
	"syscall"

	"github.com/sourcegraph/ctxvfs"
)

var drives []string

func init() {
	// loading logical drive names
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		panic(err)
	}
	getLogicalDrivesHandle, err := syscall.GetProcAddress(kernel32, "GetLogicalDrives")
	if err != nil {
		panic(err)
	}

	if ret, _, callErr := syscall.Syscall(uintptr(getLogicalDrivesHandle), 0, 0, 0, 0); callErr != 0 {
		panic(callErr)
	} else {
		drives = bitsToDrives(uint32(ret))
	}
}

// bindLocalFs implementation for Windows binds local FS rooted at drive name
// for every logical drive name using empty prefix allowing to read data from any
// logical disk mimicking Unix's
//    ns.Bind("/", ctxvfs.OS("/"), "/", mode)
func bindLocalFs(ns ctxvfs.NameSpace, mode ctxvfs.BindMode) {
	for _, drive := range drives {
		ns.Bind(drive, ctxvfs.OS(drive), "", mode)
	}
}

// bindFs implementation for Windows binds given FS
// for every logical drive name using drive name as a prefix allowing to read data from any
// logical disk mimicking Unix's
//    ns.Bind("/", newfs, "/", mode)
// As a fallback option, bindFs still binds to "/"
func bindFs(ns ctxvfs.NameSpace, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	for _, drive := range drives {
		ns.Bind(drive, newfs, drive, mode)
	}
	// fallback
	ns.Bind("/", newfs, "/", mode)
}

// bindPath binds file system to path's drive name as a prefix
func bindPath(path string, ns ctxvfs.NameSpace, newfs ctxvfs.FileSystem, mode ctxvfs.BindMode) {
	volume := filepath.VolumeName(path)
	if volume != "" {
		volume += ":"
	}
	ns.Bind(path, newfs, volume, mode)
}

// bitsToDrives converts syscall response to array of logical drive names (lowercase)
func bitsToDrives(bitMap uint32) []string {

	i := 0
	for i < 32 && bitMap != 0 {
		if bitMap&1 == 1 {
			drives = append(drives, string('a'+i)+":")
		}
		bitMap >>= 1
		i++
	}

	return drives
}
