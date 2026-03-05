//go:build fuse3
// +build fuse3

package aferofuse

import (
	"github.com/winfsp/cgofuse/fuse"
)

// compile-time assertions
var (
	_ fuse.FileSystemChmod3  = (*AferoFs)(nil)
	_ fuse.FileSystemChown3  = (*AferoFs)(nil)
	_ fuse.FileSystemUtimens3 = (*AferoFs)(nil)
	_ fuse.FileSystemRename3 = (*AferoFs)(nil)
)

// Chmod3 changes file permissions (FUSE3 variant with file handle).
func (afs *AferoFs) Chmod3(path string, mode uint32, fh uint64) int {
	errc := afs.Chmod(path, mode)
	afs.debug("Chmod3", "path", path, "mode", mode, "fh", fh, "errc", errc)
	return errc
}

// Chown3 changes file ownership (FUSE3 variant with file handle).
func (afs *AferoFs) Chown3(path string, uid uint32, gid uint32, fh uint64) int {
	errc := afs.Chown(path, uid, gid)
	afs.debug("Chown3", "path", path, "uid", uid, "gid", gid, "fh", fh, "errc", errc)
	return errc
}

// Utimens3 changes file times (FUSE3 variant with file handle).
func (afs *AferoFs) Utimens3(path string, tmsp []fuse.Timespec, fh uint64) int {
	errc := afs.Utimens(path, tmsp)
	afs.debug("Utimens3", "path", path, "fh", fh, "errc", errc)
	return errc
}

// Rename3 renames a file or directory (FUSE3 variant with flags).
//
// Supported flags: none. RENAME_NOREPLACE, RENAME_EXCHANGE, and
// RENAME_WHITEOUT are not supported by afero and will return -EINVAL.
func (afs *AferoFs) Rename3(oldpath string, newpath string, flags uint32) int {
	if flags != 0 {
		afs.debug("Rename3", "old", oldpath, "new", newpath, "flags", flags, "errc", -fuse.EINVAL)
		return -fuse.EINVAL
	}
	errc := afs.Rename(oldpath, newpath)
	afs.debug("Rename3", "old", oldpath, "new", newpath, "flags", flags, "errc", errc)
	return errc
}
