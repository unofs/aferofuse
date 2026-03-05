package aferofuse

import (
	"errors"
	"os"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
)

// fuseErrc converts a Go error to a negative FUSE error code.
// Returns 0 if err is nil.
func fuseErrc(err error) int {
	if err == nil {
		return 0
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		return -int(errno)
	}

	switch {
	case errors.Is(err, os.ErrNotExist):
		return -fuse.ENOENT
	case errors.Is(err, os.ErrExist):
		return -fuse.EEXIST
	case errors.Is(err, os.ErrPermission):
		return -fuse.EACCES
	case errors.Is(err, os.ErrClosed):
		return -fuse.EBADF
	case errors.Is(err, os.ErrInvalid):
		return -fuse.EINVAL
	default:
		return -fuse.EIO
	}
}
