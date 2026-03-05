package aferofuse

import (
	"errors"
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/winfsp/cgofuse/fuse"
)

// AferoFs implements fuse.FileSystemInterface backed by an afero.Fs.
type AferoFs struct {
	fuse.FileSystemBase
	fs      afero.Fs
	handles *handleTable
	uid     uint32
	gid     uint32
}

// compile-time assertion
var _ fuse.FileSystemInterface = (*AferoFs)(nil)

// New creates a new AferoFs wrapping the given afero filesystem.
func New(fs afero.Fs) *AferoFs {
	return &AferoFs{
		fs:      fs,
		handles: newHandleTable(),
	}
}

// Init is called when the filesystem is mounted.
func (afs *AferoFs) Init() {
	uid, gid, _ := fuse.Getcontext()
	afs.uid = uid
	afs.gid = gid
}

// Destroy is called when the filesystem is unmounted.
func (afs *AferoFs) Destroy() {
}

// Getattr gets file attributes.
func (afs *AferoFs) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if fh != ^uint64(0) {
		f := afs.handles.Get(fh)
		if f != nil {
			fi, err := f.Stat()
			if err != nil {
				return fuseErrc(err)
			}
			fillStat(stat, fi, afs.uid, afs.gid)
			return 0
		}
	}

	fi, err := afs.fs.Stat(path)
	if err != nil {
		return fuseErrc(err)
	}
	fillStat(stat, fi, afs.uid, afs.gid)
	return 0
}

// Mkdir creates a directory.
func (afs *AferoFs) Mkdir(path string, mode uint32) int {
	return fuseErrc(afs.fs.Mkdir(path, posixToFileMode(mode)))
}

// Rmdir removes a directory.
func (afs *AferoFs) Rmdir(path string) int {
	fi, err := afs.fs.Stat(path)
	if err != nil {
		return fuseErrc(err)
	}
	if !fi.IsDir() {
		return -fuse.ENOTDIR
	}
	return fuseErrc(afs.fs.Remove(path))
}

// Unlink removes a file.
func (afs *AferoFs) Unlink(path string) int {
	fi, err := afs.fs.Stat(path)
	if err != nil {
		return fuseErrc(err)
	}
	if fi.IsDir() {
		return -fuse.EISDIR
	}
	return fuseErrc(afs.fs.Remove(path))
}

// Rename renames a file or directory.
func (afs *AferoFs) Rename(oldpath string, newpath string) int {
	return fuseErrc(afs.fs.Rename(oldpath, newpath))
}

// Chmod changes file permissions.
func (afs *AferoFs) Chmod(path string, mode uint32) int {
	return fuseErrc(afs.fs.Chmod(path, posixToFileMode(mode)))
}

// Chown changes file ownership.
func (afs *AferoFs) Chown(path string, uid uint32, gid uint32) int {
	uidInt := int(uid)
	gidInt := int(gid)
	if uid == ^uint32(0) {
		uidInt = -1
	}
	if gid == ^uint32(0) {
		gidInt = -1
	}
	return fuseErrc(afs.fs.Chown(path, uidInt, gidInt))
}

// Utimens changes file access and modification times.
func (afs *AferoFs) Utimens(path string, tmsp []fuse.Timespec) int {
	return fuseErrc(afs.fs.Chtimes(path, tmsp[0].Time(), tmsp[1].Time()))
}

// Open opens a file.
func (afs *AferoFs) Open(path string, flags int) (int, uint64) {
	f, err := afs.fs.OpenFile(path, fuseOpenFlagsToOs(flags), 0)
	if err != nil {
		return fuseErrc(err), ^uint64(0)
	}
	return 0, afs.handles.Allocate(f)
}

// Create creates and opens a file.
func (afs *AferoFs) Create(path string, flags int, mode uint32) (int, uint64) {
	f, err := afs.fs.OpenFile(path, fuseOpenFlagsToOs(flags)|os.O_CREATE, posixToFileMode(mode))
	if err != nil {
		return fuseErrc(err), ^uint64(0)
	}
	return 0, afs.handles.Allocate(f)
}

// Read reads data from an open file.
func (afs *AferoFs) Read(path string, buff []byte, ofst int64, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		return -fuse.EBADF
	}
	n, err := f.ReadAt(buff, ofst)
	if n > 0 {
		return n
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return fuseErrc(err)
	}
	return 0
}

// Write writes data to an open file.
func (afs *AferoFs) Write(path string, buff []byte, ofst int64, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		return -fuse.EBADF
	}
	n, err := f.WriteAt(buff, ofst)
	if err != nil && n == 0 {
		return fuseErrc(err)
	}
	return n
}

// Truncate changes the size of a file.
func (afs *AferoFs) Truncate(path string, size int64, fh uint64) int {
	if fh != ^uint64(0) {
		f := afs.handles.Get(fh)
		if f != nil {
			return fuseErrc(f.Truncate(size))
		}
	}

	f, err := afs.fs.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fuseErrc(err)
	}
	err = f.Truncate(size)
	f.Close()
	return fuseErrc(err)
}

// Release closes an open file.
func (afs *AferoFs) Release(path string, fh uint64) int {
	f := afs.handles.Release(fh)
	if f != nil {
		f.Close()
	}
	return 0
}

// Flush is called on each close of an open file.
func (afs *AferoFs) Flush(path string, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		return -fuse.EBADF
	}
	return fuseErrc(f.Sync())
}

// Fsync synchronizes file contents.
func (afs *AferoFs) Fsync(path string, datasync bool, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		return -fuse.EBADF
	}
	return fuseErrc(f.Sync())
}

// Opendir opens a directory.
func (afs *AferoFs) Opendir(path string) (int, uint64) {
	f, err := afs.fs.Open(path)
	if err != nil {
		return fuseErrc(err), ^uint64(0)
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fuseErrc(err), ^uint64(0)
	}
	if !fi.IsDir() {
		f.Close()
		return -fuse.ENOTDIR, ^uint64(0)
	}

	return 0, afs.handles.Allocate(f)
}

// Readdir reads directory entries.
func (afs *AferoFs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64, fh uint64) int {

	f := afs.handles.Get(fh)
	if f == nil {
		return -fuse.EBADF
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	entries, err := f.Readdir(-1)
	if err != nil {
		return fuseErrc(err)
	}

	for _, entry := range entries {
		var stat fuse.Stat_t
		fillStat(&stat, entry, afs.uid, afs.gid)
		if !fill(entry.Name(), &stat, 0) {
			break
		}
	}
	return 0
}

// Releasedir closes an open directory.
func (afs *AferoFs) Releasedir(path string, fh uint64) int {
	f := afs.handles.Release(fh)
	if f != nil {
		f.Close()
	}
	return 0
}

// Statfs gets filesystem statistics.
func (afs *AferoFs) Statfs(path string, stat *fuse.Statfs_t) int {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1 << 20
	stat.Bfree = 1 << 19
	stat.Bavail = 1 << 19
	stat.Files = 1 << 20
	stat.Ffree = 1 << 19
	stat.Favail = 1 << 19
	stat.Namemax = 255
	return 0
}

// Access checks file access permissions.
func (afs *AferoFs) Access(path string, mask uint32) int {
	_, err := afs.fs.Stat(path)
	return fuseErrc(err)
}
