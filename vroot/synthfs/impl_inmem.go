package synthfs

import (
	"io/fs"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

var _ FileViewAllocator = (*MemFileAllocator)(nil)

type MemFileAllocator struct {
	clock clock.WallClock
}

func NewMemFileAllocator(clock clock.WallClock) *MemFileAllocator {
	return &MemFileAllocator{
		clock: clock,
	}
}

func (m *MemFileAllocator) Allocate(path string, perm fs.FileMode) FileView {
	return &memFileData{
		path: path,
		file: newMemFile(perm.Perm(), m.clock),
	}
}

var _ FileView = (*memFileData)(nil)

type memFileData struct {
	path string
	file *memFile
}

func (m *memFileData) Close() error {
	// currently nothing
	return nil
}

func (m *memFileData) Open(flag int) (vroot.File, error) {
	return newMemFileHandle(m.file, m.path, flag), nil
}
