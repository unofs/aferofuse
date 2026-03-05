package aferofuse

import (
	"os"

	"github.com/winfsp/cgofuse/fuse"
)

// fileModeToPosix converts Go's os.FileMode to a POSIX mode uint32.
func fileModeToPosix(m os.FileMode) uint32 {
	mode := uint32(m.Perm())

	if m&os.ModeSetuid != 0 {
		mode |= fuse.S_ISUID
	}
	if m&os.ModeSetgid != 0 {
		mode |= fuse.S_ISGID
	}
	if m&os.ModeSticky != 0 {
		mode |= fuse.S_ISVTX
	}

	switch {
	case m.IsDir():
		mode |= fuse.S_IFDIR
	case m&os.ModeSymlink != 0:
		mode |= fuse.S_IFLNK
	case m&os.ModeNamedPipe != 0:
		mode |= fuse.S_IFIFO
	case m&os.ModeSocket != 0:
		mode |= fuse.S_IFSOCK
	case m&os.ModeDevice != 0:
		if m&os.ModeCharDevice != 0 {
			mode |= fuse.S_IFCHR
		} else {
			mode |= fuse.S_IFBLK
		}
	default:
		mode |= fuse.S_IFREG
	}

	return mode
}

// posixToFileMode converts POSIX mode permission and special bits to Go's os.FileMode.
func posixToFileMode(mode uint32) os.FileMode {
	fm := os.FileMode(mode & 0777)

	if mode&fuse.S_ISUID != 0 {
		fm |= os.ModeSetuid
	}
	if mode&fuse.S_ISGID != 0 {
		fm |= os.ModeSetgid
	}
	if mode&fuse.S_ISVTX != 0 {
		fm |= os.ModeSticky
	}

	return fm
}

// fillStat populates a fuse.Stat_t from an os.FileInfo.
func fillStat(stat *fuse.Stat_t, fi os.FileInfo, uid, gid uint32) {
	stat.Mode = fileModeToPosix(fi.Mode())
	stat.Size = fi.Size()

	if fi.IsDir() {
		stat.Nlink = 2
	} else {
		stat.Nlink = 1
	}

	stat.Uid = uid
	stat.Gid = gid

	ts := fuse.NewTimespec(fi.ModTime())
	stat.Atim = ts
	stat.Mtim = ts
	stat.Ctim = ts
	stat.Birthtim = ts

	if fi.Size() > 0 {
		stat.Blksize = 4096
		stat.Blocks = (fi.Size() + 511) / 512
	}
}

// fuseOpenFlagsToOs converts FUSE open flags to Go os package flags.
func fuseOpenFlagsToOs(fuseFlags int) int {
	var flags int

	switch fuseFlags & fuse.O_ACCMODE {
	case fuse.O_RDONLY:
		flags = os.O_RDONLY
	case fuse.O_WRONLY:
		flags = os.O_WRONLY
	case fuse.O_RDWR:
		flags = os.O_RDWR
	}

	if fuseFlags&fuse.O_APPEND != 0 {
		flags |= os.O_APPEND
	}
	if fuseFlags&fuse.O_CREAT != 0 {
		flags |= os.O_CREATE
	}
	if fuseFlags&fuse.O_EXCL != 0 {
		flags |= os.O_EXCL
	}
	if fuseFlags&fuse.O_TRUNC != 0 {
		flags |= os.O_TRUNC
	}

	return flags
}
