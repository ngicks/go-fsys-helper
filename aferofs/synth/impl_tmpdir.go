package synth

import (
	"io/fs"
	"os"
	"path"
	"sync"

	"github.com/spf13/afero"
)

var _ FileViewAllocator = (*tmpDirAllocator)(nil)

type tmpDirAllocator struct {
	fsys    afero.Fs
	pattern string
}

func NewTempDirAllocator(fsys afero.Fs, pattern string) FileViewAllocator {
	return &tmpDirAllocator{
		fsys:    fsys,
		pattern: pattern,
	}
}

// Allocate implements FileDataAllocator.
func (t *tmpDirAllocator) Allocate(path string, perm fs.FileMode) FileView {
	return newTmpDirFileView(t.fsys, t.pattern)
}

var _ FileView = (*tmpDirFileView)(nil)

type tmpDirFileView struct {
	mu      sync.Mutex
	fsys    afero.Fs
	pattern string
	path    string
}

func newTmpDirFileView(fsys afero.Fs, pattern string) FileView {
	return &tmpDirFileView{
		fsys:    fsys,
		pattern: pattern,
	}
}

func (b *tmpDirFileView) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.path != "" {
		b.path = ""
		return b.fsys.Remove(b.path)
	}
	return nil
}

func (b *tmpDirFileView) create() error {
	if b.path != "" {
		return nil
	}
	f, err := afero.TempFile(b.fsys, ".", b.pattern)
	if err != nil {
		return err
	}
	s, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return err
	}
	b.path = path.Base(s.Name())
	return nil
}

func (b *tmpDirFileView) Open(flag int) (afero.File, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.create(); err != nil {
		return nil, err
	}

	f, err := b.fsys.OpenFile(b.path, flag, 0)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (b *tmpDirFileView) Stat() (fs.FileInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.create(); err != nil {
		return nil, err
	}

	f, err := b.fsys.Open(b.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return f.Stat()
}

func (b *tmpDirFileView) Truncate(size int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.create(); err != nil {
		return err
	}

	f, err := b.fsys.OpenFile(b.path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Truncate(size)
}

func (b *tmpDirFileView) Rename(newname string) {
	//
}
