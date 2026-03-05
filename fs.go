package aferofuse

import (
	"errors"
	"io"
	"log/slog"
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
	logger  *slog.Logger
}

// compile-time assertion
var _ fuse.FileSystemInterface = (*AferoFs)(nil)

// New creates a new AferoFs wrapping the given afero filesystem.
// If logger is nil, no debug logging is performed.
func New(fs afero.Fs, logger *slog.Logger) *AferoFs {
	return &AferoFs{
		fs:      fs,
		handles: newHandleTable(),
		logger:  logger,
	}
}

func (afs *AferoFs) debug(op string, args ...any) {
	if afs.logger != nil {
		afs.logger.Debug(op, args...)
	}
}

// Init is called when the filesystem is mounted.
func (afs *AferoFs) Init() {
	uid, gid, _ := fuse.Getcontext()
	afs.uid = uid
	afs.gid = gid
	afs.debug("Init", "uid", uid, "gid", gid)
}

// Destroy is called when the filesystem is unmounted.
func (afs *AferoFs) Destroy() {
	afs.debug("Destroy")
}

// Getattr gets file attributes.
func (afs *AferoFs) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if fh != ^uint64(0) {
		f := afs.handles.Get(fh)
		if f != nil {
			fi, err := f.Stat()
			if err != nil {
				errc := fuseErrc(err)
				afs.debug("Getattr", "path", path, "fh", fh, "errc", errc)
				return errc
			}
			fillStat(stat, fi, afs.uid, afs.gid)
			afs.debug("Getattr", "path", path, "fh", fh, "size", stat.Size, "mode", stat.Mode, "errc", 0)
			return 0
		}
	}

	fi, err := afs.fs.Stat(path)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Getattr", "path", path, "errc", errc)
		return errc
	}
	fillStat(stat, fi, afs.uid, afs.gid)
	afs.debug("Getattr", "path", path, "size", stat.Size, "mode", stat.Mode, "errc", 0)
	return 0
}

// Mkdir creates a directory.
func (afs *AferoFs) Mkdir(path string, mode uint32) int {
	errc := fuseErrc(afs.fs.Mkdir(path, posixToFileMode(mode)))
	afs.debug("Mkdir", "path", path, "mode", mode, "errc", errc)
	return errc
}

// Rmdir removes a directory.
func (afs *AferoFs) Rmdir(path string) int {
	fi, err := afs.fs.Stat(path)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Rmdir", "path", path, "errc", errc)
		return errc
	}
	if !fi.IsDir() {
		afs.debug("Rmdir", "path", path, "errc", -fuse.ENOTDIR)
		return -fuse.ENOTDIR
	}
	errc := fuseErrc(afs.fs.Remove(path))
	afs.debug("Rmdir", "path", path, "errc", errc)
	return errc
}

// Unlink removes a file.
func (afs *AferoFs) Unlink(path string) int {
	fi, err := afs.fs.Stat(path)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Unlink", "path", path, "errc", errc)
		return errc
	}
	if fi.IsDir() {
		afs.debug("Unlink", "path", path, "errc", -fuse.EISDIR)
		return -fuse.EISDIR
	}
	errc := fuseErrc(afs.fs.Remove(path))
	afs.debug("Unlink", "path", path, "errc", errc)
	return errc
}

// Rename renames a file or directory.
func (afs *AferoFs) Rename(oldpath string, newpath string) int {
	errc := fuseErrc(afs.fs.Rename(oldpath, newpath))
	afs.debug("Rename", "old", oldpath, "new", newpath, "errc", errc)
	return errc
}

// Chmod changes file permissions.
func (afs *AferoFs) Chmod(path string, mode uint32) int {
	errc := fuseErrc(afs.fs.Chmod(path, posixToFileMode(mode)))
	afs.debug("Chmod", "path", path, "mode", mode, "errc", errc)
	return errc
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
	errc := fuseErrc(afs.fs.Chown(path, uidInt, gidInt))
	afs.debug("Chown", "path", path, "uid", uidInt, "gid", gidInt, "errc", errc)
	return errc
}

// Utimens changes file access and modification times.
func (afs *AferoFs) Utimens(path string, tmsp []fuse.Timespec) int {
	errc := fuseErrc(afs.fs.Chtimes(path, tmsp[0].Time(), tmsp[1].Time()))
	afs.debug("Utimens", "path", path, "errc", errc)
	return errc
}

// Open opens a file.
func (afs *AferoFs) Open(path string, flags int) (int, uint64) {
	f, err := afs.fs.OpenFile(path, fuseOpenFlagsToOs(flags), 0)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Open", "path", path, "flags", flags, "errc", errc)
		return errc, ^uint64(0)
	}
	fh := afs.handles.Allocate(f)
	afs.debug("Open", "path", path, "flags", flags, "fh", fh, "errc", 0)
	return 0, fh
}

// Create creates and opens a file.
func (afs *AferoFs) Create(path string, flags int, mode uint32) (int, uint64) {
	f, err := afs.fs.OpenFile(path, fuseOpenFlagsToOs(flags)|os.O_CREATE, posixToFileMode(mode))
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Create", "path", path, "flags", flags, "mode", mode, "errc", errc)
		return errc, ^uint64(0)
	}
	fh := afs.handles.Allocate(f)
	afs.debug("Create", "path", path, "flags", flags, "mode", mode, "fh", fh, "errc", 0)
	return 0, fh
}

// Read reads data from an open file.
func (afs *AferoFs) Read(path string, buff []byte, ofst int64, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		afs.debug("Read", "path", path, "fh", fh, "errc", -fuse.EBADF)
		return -fuse.EBADF
	}
	n, err := f.ReadAt(buff, ofst)
	if n > 0 {
		afs.debug("Read", "path", path, "ofst", ofst, "len", len(buff), "fh", fh, "n", n)
		return n
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		errc := fuseErrc(err)
		afs.debug("Read", "path", path, "ofst", ofst, "len", len(buff), "fh", fh, "errc", errc)
		return errc
	}
	afs.debug("Read", "path", path, "ofst", ofst, "len", len(buff), "fh", fh, "n", 0)
	return 0
}

// Write writes data to an open file.
func (afs *AferoFs) Write(path string, buff []byte, ofst int64, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		afs.debug("Write", "path", path, "fh", fh, "errc", -fuse.EBADF)
		return -fuse.EBADF
	}
	n, err := f.WriteAt(buff, ofst)
	if err != nil && n == 0 {
		errc := fuseErrc(err)
		afs.debug("Write", "path", path, "ofst", ofst, "len", len(buff), "fh", fh, "errc", errc)
		return errc
	}
	afs.debug("Write", "path", path, "ofst", ofst, "len", len(buff), "fh", fh, "n", n)
	return n
}

// Truncate changes the size of a file.
func (afs *AferoFs) Truncate(path string, size int64, fh uint64) int {
	if fh != ^uint64(0) {
		f := afs.handles.Get(fh)
		if f != nil {
			errc := fuseErrc(f.Truncate(size))
			afs.debug("Truncate", "path", path, "size", size, "fh", fh, "errc", errc)
			return errc
		}
	}

	f, err := afs.fs.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Truncate", "path", path, "size", size, "errc", errc)
		return errc
	}
	err = f.Truncate(size)
	f.Close()
	errc := fuseErrc(err)
	afs.debug("Truncate", "path", path, "size", size, "errc", errc)
	return errc
}

// Release closes an open file.
func (afs *AferoFs) Release(path string, fh uint64) int {
	f := afs.handles.Release(fh)
	if f != nil {
		f.Close()
	}
	afs.debug("Release", "path", path, "fh", fh)
	return 0
}

// Flush is called on each close of an open file.
func (afs *AferoFs) Flush(path string, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		afs.debug("Flush", "path", path, "fh", fh, "errc", -fuse.EBADF)
		return -fuse.EBADF
	}
	errc := fuseErrc(f.Sync())
	afs.debug("Flush", "path", path, "fh", fh, "errc", errc)
	return errc
}

// Fsync synchronizes file contents.
func (afs *AferoFs) Fsync(path string, datasync bool, fh uint64) int {
	f := afs.handles.Get(fh)
	if f == nil {
		afs.debug("Fsync", "path", path, "fh", fh, "errc", -fuse.EBADF)
		return -fuse.EBADF
	}
	errc := fuseErrc(f.Sync())
	afs.debug("Fsync", "path", path, "datasync", datasync, "fh", fh, "errc", errc)
	return errc
}

// Opendir opens a directory.
func (afs *AferoFs) Opendir(path string) (int, uint64) {
	f, err := afs.fs.Open(path)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Opendir", "path", path, "errc", errc)
		return errc, ^uint64(0)
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		errc := fuseErrc(err)
		afs.debug("Opendir", "path", path, "errc", errc)
		return errc, ^uint64(0)
	}
	if !fi.IsDir() {
		f.Close()
		afs.debug("Opendir", "path", path, "errc", -fuse.ENOTDIR)
		return -fuse.ENOTDIR, ^uint64(0)
	}

	fh := afs.handles.Allocate(f)
	afs.debug("Opendir", "path", path, "fh", fh, "errc", 0)
	return 0, fh
}

// Readdir reads directory entries.
func (afs *AferoFs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64, fh uint64) int {

	f := afs.handles.Get(fh)
	if f == nil {
		afs.debug("Readdir", "path", path, "fh", fh, "errc", -fuse.EBADF)
		return -fuse.EBADF
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	entries, err := f.Readdir(-1)
	if err != nil {
		errc := fuseErrc(err)
		afs.debug("Readdir", "path", path, "fh", fh, "errc", errc)
		return errc
	}

	for _, entry := range entries {
		var stat fuse.Stat_t
		fillStat(&stat, entry, afs.uid, afs.gid)
		if !fill(entry.Name(), &stat, 0) {
			break
		}
	}
	afs.debug("Readdir", "path", path, "fh", fh, "entries", len(entries)+2, "errc", 0)
	return 0
}

// Releasedir closes an open directory.
func (afs *AferoFs) Releasedir(path string, fh uint64) int {
	f := afs.handles.Release(fh)
	if f != nil {
		f.Close()
	}
	afs.debug("Releasedir", "path", path, "fh", fh)
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
	afs.debug("Statfs", "path", path, "errc", 0)
	return 0
}

// Access checks file access permissions.
func (afs *AferoFs) Access(path string, mask uint32) int {
	_, err := afs.fs.Stat(path)
	errc := fuseErrc(err)
	afs.debug("Access", "path", path, "mask", mask, "errc", errc)
	return errc
}
