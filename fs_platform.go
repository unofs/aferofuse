package aferofuse

import (
	"github.com/winfsp/cgofuse/fuse"
)

// compile-time assertions
var (
	_ fuse.FileSystemChflags   = (*AferoFs)(nil)
	_ fuse.FileSystemSetcrtime = (*AferoFs)(nil)
	_ fuse.FileSystemSetchgtime = (*AferoFs)(nil)
)

// Chflags changes the BSD file flags (Windows file attributes). [OSX and Windows only]
//
// afero does not support BSD flags, so this is a no-op that returns -ENOSYS.
func (afs *AferoFs) Chflags(path string, flags uint32) int {
	afs.debug("Chflags", "path", path, "flags", flags, "errc", -fuse.ENOSYS)
	return -fuse.ENOSYS
}

// Setcrtime changes the file creation (birth) time. [OSX and Windows only]
//
// afero.Fs.Chtimes only supports atime and mtime, so creation time cannot be
// set. This is a no-op that returns -ENOSYS.
func (afs *AferoFs) Setcrtime(path string, tmsp fuse.Timespec) int {
	afs.debug("Setcrtime", "path", path, "errc", -fuse.ENOSYS)
	return -fuse.ENOSYS
}

// Setchgtime changes the file change (ctime) time. [OSX and Windows only]
//
// afero.Fs.Chtimes only supports atime and mtime, so ctime cannot be set.
// This is a no-op that returns -ENOSYS.
func (afs *AferoFs) Setchgtime(path string, tmsp fuse.Timespec) int {
	afs.debug("Setchgtime", "path", path, "errc", -fuse.ENOSYS)
	return -fuse.ENOSYS
}
