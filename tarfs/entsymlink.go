package tarfs

import (
	"io"
	"path"
)

type symlink struct {
	h *Section
}

func (s *symlink) header() *Section {
	return s.h
}

func (s *symlink) open(r io.ReaderAt, path string) openDirentry {
	// Symlinks should not be opened directly - they should be resolved first
	panic("symlink.open() should not be called - symlinks should be resolved before opening")
}

func (s *symlink) readLink() (string, error) {
	return path.Clean(s.h.h.Linkname), nil
}
