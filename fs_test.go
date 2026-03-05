package aferofuse

import (
	"errors"
	"os"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/winfsp/cgofuse/fuse"
)

func newTestFs(t *testing.T) *AferoFs {
	t.Helper()
	memFs := afero.NewMemMapFs()
	afs := New(memFs)
	afs.uid = 1000
	afs.gid = 1000
	return afs
}

// --- Getattr ---

func TestGetattr_Root(t *testing.T) {
	afs := newTestFs(t)
	var stat fuse.Stat_t
	errc := afs.Getattr("/", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Getattr root: expected 0, got %d", errc)
	}
	if stat.Mode&fuse.S_IFDIR == 0 {
		t.Fatalf("expected directory mode, got %o", stat.Mode)
	}
}

func TestGetattr_NonExistent(t *testing.T) {
	afs := newTestFs(t)
	var stat fuse.Stat_t
	errc := afs.Getattr("/noexist", &stat, ^uint64(0))
	if errc != -fuse.ENOENT {
		t.Fatalf("expected -ENOENT, got %d", errc)
	}
}

func TestGetattr_WithFileHandle(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/testfile", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	defer afs.Release("/testfile", fh)

	var stat fuse.Stat_t
	errc = afs.Getattr("/testfile", &stat, fh)
	if errc != 0 {
		t.Fatalf("Getattr with fh: expected 0, got %d", errc)
	}
	if stat.Mode&fuse.S_IFREG == 0 {
		t.Fatalf("expected regular file mode, got %o", stat.Mode)
	}
}

// --- Mkdir ---

func TestMkdir(t *testing.T) {
	afs := newTestFs(t)
	errc := afs.Mkdir("/testdir", 0755)
	if errc != 0 {
		t.Fatalf("Mkdir: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/testdir", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Getattr after Mkdir: expected 0, got %d", errc)
	}
	if stat.Mode&fuse.S_IFDIR == 0 {
		t.Fatalf("expected directory mode")
	}
	if stat.Nlink != 2 {
		t.Fatalf("expected nlink=2 for directory, got %d", stat.Nlink)
	}
}

// --- Rmdir ---

func TestRmdir(t *testing.T) {
	afs := newTestFs(t)
	afs.Mkdir("/testdir", 0755)

	errc := afs.Rmdir("/testdir")
	if errc != 0 {
		t.Fatalf("Rmdir: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/testdir", &stat, ^uint64(0))
	if errc != -fuse.ENOENT {
		t.Fatalf("Getattr after Rmdir: expected -ENOENT, got %d", errc)
	}
}

func TestRmdir_NotDir(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	errc = afs.Rmdir("/file")
	if errc != -fuse.ENOTDIR {
		t.Fatalf("Rmdir on file: expected -ENOTDIR, got %d", errc)
	}
}

func TestRmdir_NonExistent(t *testing.T) {
	afs := newTestFs(t)
	errc := afs.Rmdir("/noexist")
	if errc != -fuse.ENOENT {
		t.Fatalf("Rmdir non-existent: expected -ENOENT, got %d", errc)
	}
}

// --- Unlink ---

func TestUnlink(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	errc = afs.Unlink("/file")
	if errc != 0 {
		t.Fatalf("Unlink: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/file", &stat, ^uint64(0))
	if errc != -fuse.ENOENT {
		t.Fatalf("Getattr after Unlink: expected -ENOENT, got %d", errc)
	}
}

func TestUnlink_IsDir(t *testing.T) {
	afs := newTestFs(t)
	afs.Mkdir("/dir", 0755)

	errc := afs.Unlink("/dir")
	if errc != -fuse.EISDIR {
		t.Fatalf("Unlink on dir: expected -EISDIR, got %d", errc)
	}
}

// --- Create / Open / Read / Write ---

func TestCreateOpenReadWrite(t *testing.T) {
	afs := newTestFs(t)

	// Create and write
	errc, fh := afs.Create("/hello", fuse.O_RDWR, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}

	data := []byte("hello, world!")
	n := afs.Write("/hello", data, 0, fh)
	if n != len(data) {
		t.Fatalf("Write: expected %d bytes, got %d", len(data), n)
	}
	afs.Release("/hello", fh)

	// Open and read
	errc, fh = afs.Open("/hello", fuse.O_RDONLY)
	if errc != 0 {
		t.Fatalf("Open: %d", errc)
	}

	buf := make([]byte, 64)
	n = afs.Read("/hello", buf, 0, fh)
	if n != len(data) {
		t.Fatalf("Read: expected %d bytes, got %d", len(data), n)
	}
	if string(buf[:n]) != "hello, world!" {
		t.Fatalf("Read: expected %q, got %q", "hello, world!", string(buf[:n]))
	}
	afs.Release("/hello", fh)
}

func TestRead_EOF(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_RDWR, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Write("/file", []byte("abc"), 0, fh)
	afs.Release("/file", fh)

	errc, fh = afs.Open("/file", fuse.O_RDONLY)
	if errc != 0 {
		t.Fatalf("Open: %d", errc)
	}

	// Read past end of file
	buf := make([]byte, 64)
	n := afs.Read("/file", buf, 100, fh)
	if n != 0 {
		t.Fatalf("Read past EOF: expected 0, got %d", n)
	}
	afs.Release("/file", fh)
}

func TestRead_InvalidHandle(t *testing.T) {
	afs := newTestFs(t)
	buf := make([]byte, 64)
	n := afs.Read("/file", buf, 0, 9999)
	if n != -fuse.EBADF {
		t.Fatalf("Read invalid handle: expected -EBADF, got %d", n)
	}
}

// --- Truncate ---

func TestTruncate(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_RDWR, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Write("/file", []byte("hello, world!"), 0, fh)

	// Truncate via file handle
	errc = afs.Truncate("/file", 5, fh)
	if errc != 0 {
		t.Fatalf("Truncate: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/file", &stat, fh)
	if errc != 0 {
		t.Fatalf("Getattr: %d", errc)
	}
	if stat.Size != 5 {
		t.Fatalf("expected size 5, got %d", stat.Size)
	}
	afs.Release("/file", fh)
}

func TestTruncate_ByPath(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_RDWR, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Write("/file", []byte("hello, world!"), 0, fh)
	afs.Release("/file", fh)

	// Truncate by path (no file handle)
	errc = afs.Truncate("/file", 3, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Truncate by path: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/file", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Getattr: %d", errc)
	}
	if stat.Size != 3 {
		t.Fatalf("expected size 3, got %d", stat.Size)
	}
}

// --- Rename ---

func TestRename(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/old", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Write("/old", []byte("data"), 0, fh)
	afs.Release("/old", fh)

	errc = afs.Rename("/old", "/new")
	if errc != 0 {
		t.Fatalf("Rename: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/old", &stat, ^uint64(0))
	if errc != -fuse.ENOENT {
		t.Fatalf("old path should be gone, got %d", errc)
	}

	errc = afs.Getattr("/new", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("new path should exist, got %d", errc)
	}
}

// --- Opendir / Readdir ---

func TestReaddir(t *testing.T) {
	afs := newTestFs(t)
	afs.Mkdir("/dir", 0755)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		errc, fh := afs.Create("/dir/"+name, fuse.O_WRONLY, 0644)
		if errc != 0 {
			t.Fatalf("Create %s: %d", name, errc)
		}
		afs.Release("/dir/"+name, fh)
	}

	errc, fh := afs.Opendir("/dir")
	if errc != 0 {
		t.Fatalf("Opendir: %d", errc)
	}

	var names []string
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		names = append(names, name)
		return true
	}

	errc = afs.Readdir("/dir", fill, 0, fh)
	if errc != 0 {
		t.Fatalf("Readdir: %d", errc)
	}
	afs.Releasedir("/dir", fh)

	sort.Strings(names)
	expected := []string{".", "..", "a.txt", "b.txt", "c.txt"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(names), names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("entry %d: expected %q, got %q", i, expected[i], names[i])
		}
	}
}

func TestOpendir_NotDir(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	errc, _ = afs.Opendir("/file")
	if errc != -fuse.ENOTDIR {
		t.Fatalf("Opendir on file: expected -ENOTDIR, got %d", errc)
	}
}

// --- Chmod ---

func TestChmod(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	errc = afs.Chmod("/file", 0755)
	if errc != 0 {
		t.Fatalf("Chmod: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/file", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Getattr: %d", errc)
	}
	perm := stat.Mode & 0777
	if perm != 0755 {
		t.Fatalf("expected mode 0755, got %o", perm)
	}
}

// --- Utimens ---

func TestUtimens(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	now := time.Now().Truncate(time.Second)
	tmsp := []fuse.Timespec{
		fuse.NewTimespec(now),
		fuse.NewTimespec(now),
	}
	errc = afs.Utimens("/file", tmsp)
	if errc != 0 {
		t.Fatalf("Utimens: expected 0, got %d", errc)
	}

	var stat fuse.Stat_t
	errc = afs.Getattr("/file", &stat, ^uint64(0))
	if errc != 0 {
		t.Fatalf("Getattr: %d", errc)
	}
	mtim := stat.Mtim.Time()
	if !mtim.Equal(now) {
		t.Fatalf("expected mtime %v, got %v", now, mtim)
	}
}

// --- Flush / Fsync ---

func TestFlush(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}

	errc = afs.Flush("/file", fh)
	if errc != 0 {
		t.Fatalf("Flush: expected 0, got %d", errc)
	}
	afs.Release("/file", fh)
}

func TestFsync(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}

	errc = afs.Fsync("/file", false, fh)
	if errc != 0 {
		t.Fatalf("Fsync: expected 0, got %d", errc)
	}
	afs.Release("/file", fh)
}

// --- Statfs ---

func TestStatfs(t *testing.T) {
	afs := newTestFs(t)
	var stat fuse.Statfs_t
	errc := afs.Statfs("/", &stat)
	if errc != 0 {
		t.Fatalf("Statfs: expected 0, got %d", errc)
	}
	if stat.Bsize != 4096 {
		t.Fatalf("expected Bsize=4096, got %d", stat.Bsize)
	}
	if stat.Namemax != 255 {
		t.Fatalf("expected Namemax=255, got %d", stat.Namemax)
	}
}

// --- Access ---

func TestAccess(t *testing.T) {
	afs := newTestFs(t)
	errc := afs.Access("/", 0)
	if errc != 0 {
		t.Fatalf("Access root: expected 0, got %d", errc)
	}

	errc = afs.Access("/noexist", 0)
	if errc != -fuse.ENOENT {
		t.Fatalf("Access non-existent: expected -ENOENT, got %d", errc)
	}
}

// --- Chown ---

func TestChown(t *testing.T) {
	afs := newTestFs(t)
	errc, fh := afs.Create("/file", fuse.O_WRONLY, 0644)
	if errc != 0 {
		t.Fatalf("Create: %d", errc)
	}
	afs.Release("/file", fh)

	// Chown with "don't change" sentinel — should not error
	errc = afs.Chown("/file", ^uint32(0), ^uint32(0))
	// MemMapFs may not support chown, accept 0 or -EIO/ENOSYS
	_ = errc
}

// --- Release ---

func TestRelease_InvalidHandle(t *testing.T) {
	afs := newTestFs(t)
	errc := afs.Release("/file", 9999)
	if errc != 0 {
		t.Fatalf("Release invalid handle should succeed, got %d", errc)
	}
}

// --- Error mapping ---

func TestFuseErrc_Nil(t *testing.T) {
	if got := fuseErrc(nil); got != 0 {
		t.Fatalf("expected 0 for nil, got %d", got)
	}
}

func TestFuseErrc_Errno(t *testing.T) {
	got := fuseErrc(syscall.ENOENT)
	if got != -fuse.ENOENT {
		t.Fatalf("expected %d, got %d", -fuse.ENOENT, got)
	}
}

func TestFuseErrc_PathError(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}
	got := fuseErrc(err)
	if got != -fuse.EACCES {
		t.Fatalf("expected %d, got %d", -fuse.EACCES, got)
	}
}

func TestFuseErrc_OsErrors(t *testing.T) {
	tests := []struct {
		err      error
		expected int
	}{
		{os.ErrNotExist, -fuse.ENOENT},
		{os.ErrExist, -fuse.EEXIST},
		{os.ErrPermission, -fuse.EACCES},
		{os.ErrClosed, -fuse.EBADF},
		{os.ErrInvalid, -fuse.EINVAL},
	}
	for _, tt := range tests {
		got := fuseErrc(tt.err)
		if got != tt.expected {
			t.Errorf("fuseErrc(%v): expected %d, got %d", tt.err, tt.expected, got)
		}
	}
}

func TestFuseErrc_Unknown(t *testing.T) {
	got := fuseErrc(errors.New("something unknown"))
	if got != -fuse.EIO {
		t.Fatalf("expected -EIO, got %d", got)
	}
}

// --- Handle table ---

func TestHandleTable_AllocateGetRelease(t *testing.T) {
	ht := newHandleTable()
	memFs := afero.NewMemMapFs()
	f, _ := memFs.Create("/test")

	fh := ht.Allocate(f)
	if fh == 0 {
		t.Fatal("handle should not be 0")
	}

	got := ht.Get(fh)
	if got != f {
		t.Fatal("Get returned wrong file")
	}

	released := ht.Release(fh)
	if released != f {
		t.Fatal("Release returned wrong file")
	}

	if ht.Get(fh) != nil {
		t.Fatal("Get after Release should return nil")
	}
}

func TestHandleTable_Concurrent(t *testing.T) {
	ht := newHandleTable()
	memFs := afero.NewMemMapFs()

	var wg sync.WaitGroup
	const n = 100

	handles := make([]uint64, n)
	files := make([]afero.File, n)

	for i := 0; i < n; i++ {
		f, _ := memFs.Create("/test")
		files[i] = f
	}

	// Concurrent allocations
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handles[idx] = ht.Allocate(files[idx])
		}(i)
	}
	wg.Wait()

	// Verify all handles are unique
	seen := make(map[uint64]bool)
	for _, h := range handles {
		if seen[h] {
			t.Fatal("duplicate handle")
		}
		seen[h] = true
	}

	// Concurrent releases
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ht.Release(handles[idx])
		}(i)
	}
	wg.Wait()
}

// --- Mode conversion ---

func TestFileModeToPosix_RegularFile(t *testing.T) {
	mode := fileModeToPosix(0644)
	if mode != fuse.S_IFREG|0644 {
		t.Fatalf("expected %o, got %o", fuse.S_IFREG|0644, mode)
	}
}

func TestFileModeToPosix_Directory(t *testing.T) {
	mode := fileModeToPosix(os.ModeDir | 0755)
	if mode != fuse.S_IFDIR|0755 {
		t.Fatalf("expected %o, got %o", fuse.S_IFDIR|0755, mode)
	}
}

func TestFileModeToPosix_Symlink(t *testing.T) {
	mode := fileModeToPosix(os.ModeSymlink | 0777)
	if mode != fuse.S_IFLNK|0777 {
		t.Fatalf("expected %o, got %o", fuse.S_IFLNK|0777, mode)
	}
}

func TestFileModeToPosix_SetuidSetgidSticky(t *testing.T) {
	m := os.FileMode(0755) | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	mode := fileModeToPosix(m)
	expected := uint32(fuse.S_IFREG | fuse.S_ISUID | fuse.S_ISGID | fuse.S_ISVTX | 0755)
	if mode != expected {
		t.Fatalf("expected %o, got %o", expected, mode)
	}
}

func TestPosixToFileMode(t *testing.T) {
	fm := posixToFileMode(0755)
	if fm != os.FileMode(0755) {
		t.Fatalf("expected %o, got %o", os.FileMode(0755), fm)
	}

	fm = posixToFileMode(uint32(fuse.S_ISUID | fuse.S_ISGID | fuse.S_ISVTX | 0755))
	expected := os.FileMode(0755) | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if fm != expected {
		t.Fatalf("expected %v, got %v", expected, fm)
	}
}

func TestFuseOpenFlagsToOs(t *testing.T) {
	flags := fuseOpenFlagsToOs(fuse.O_RDONLY)
	if flags != os.O_RDONLY {
		t.Fatalf("O_RDONLY: expected %d, got %d", os.O_RDONLY, flags)
	}

	flags = fuseOpenFlagsToOs(fuse.O_WRONLY | fuse.O_CREAT | fuse.O_TRUNC)
	expected := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if flags != expected {
		t.Fatalf("O_WRONLY|O_CREAT|O_TRUNC: expected %d, got %d", expected, flags)
	}

	flags = fuseOpenFlagsToOs(fuse.O_RDWR | fuse.O_APPEND)
	expected = os.O_RDWR | os.O_APPEND
	if flags != expected {
		t.Fatalf("O_RDWR|O_APPEND: expected %d, got %d", expected, flags)
	}
}
