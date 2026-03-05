package aferofuse

import (
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
)

// handleTable maps uint64 file handles to afero.File objects.
// It is safe for concurrent use.
type handleTable struct {
	mu      sync.RWMutex
	handles map[uint64]afero.File
	next    uint64
}

func newHandleTable() *handleTable {
	return &handleTable{
		handles: make(map[uint64]afero.File),
	}
}

// Allocate assigns a new unique file handle to the given file.
func (ht *handleTable) Allocate(f afero.File) uint64 {
	fh := atomic.AddUint64(&ht.next, 1)
	ht.mu.Lock()
	ht.handles[fh] = f
	ht.mu.Unlock()
	return fh
}

// Get retrieves the file associated with a handle.
// Returns nil if the handle is invalid.
func (ht *handleTable) Get(fh uint64) afero.File {
	ht.mu.RLock()
	f := ht.handles[fh]
	ht.mu.RUnlock()
	return f
}

// Release removes a handle from the table and returns the associated file.
// The caller is responsible for closing the file.
// Returns nil if the handle is invalid.
func (ht *handleTable) Release(fh uint64) afero.File {
	ht.mu.Lock()
	f := ht.handles[fh]
	delete(ht.handles, fh)
	ht.mu.Unlock()
	return f
}
