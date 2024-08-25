package vmesh

import (
	"io/fs"
	"path"

	"github.com/ngicks/go-fsys-helper/aferofs/clock"
	"github.com/spf13/afero"
)

var _ FileDataAllocator = (*MemFileDataAllocator)(nil)

type MemFileDataAllocator struct {
	clock clock.WallClock
}

func NewMemFileDataAllocator(clock clock.WallClock) *MemFileDataAllocator {
	return &MemFileDataAllocator{
		clock: clock,
	}
}

func (m *MemFileDataAllocator) Allocate(path string, perm fs.FileMode) FileData {
	return &memFileData{
		path: path,
		file: newMemFile(perm.Perm(), m.clock),
	}
}

var _ FileData = (*memFileData)(nil)

type memFileData struct {
	path string
	file *memFile
}

func (m *memFileData) Close() error {
	// currently nothing
	return nil
}

func (m *memFileData) Open(flag int) (afero.File, error) {
	return newMemFileHandle(m.file, m.path, flag), nil
}

func (m *memFileData) Stat() (fs.FileInfo, error) {
	return m.file.stat(path.Base(m.path)), nil
}

func (m *memFileData) Truncate(size int64) error {
	return m.file.Truncate(size)
}
