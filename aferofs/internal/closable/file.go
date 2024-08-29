package closable

import (
	"io/fs"
	"sync"

	"github.com/spf13/afero"
)

//go:generate go run ../../cmd/implwrapper -pkg ./ -file Closable

var _ afero.File = (*Closable[afero.File])(nil)

type Closable[T afero.File] struct {
	mu     sync.RWMutex
	closed bool
	inner  afero.File
}

func NewFile[T afero.File](inner afero.File) *Closable[T] {
	return &Closable[T]{inner: inner}
}

func (f *Closable[T]) beforeEach(ms string, _ ...any) error {
	f.mu.RLock()
	if f.closed {
		f.mu.RUnlock()
		return fs.ErrClosed
	}
	f.mu.RUnlock()
	if ms == "Close" {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
	}
	return nil
}
