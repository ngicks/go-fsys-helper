package tarfs

import (
	"fmt"
	"io"
	"syscall"
)

type hardlink struct {
	h *Section
}

func (h *hardlink) header() *Section {
	return h.h
}

func (h *hardlink) open(r io.ReaderAt, path string) openDirentry {
	// Hard links should not be opened directly - they should be resolved first
	panic("hardlink.open() should not be called - hardlinks should be resolved before opening")
}

func (h *hardlink) readLink() (string, error) {
	return "", pathErr("readlink", "", syscall.EINVAL)
}

func (h *hardlink) overlayHardlink(target direntry) direntry {
	return &hardlinkOverlay{h, target}
}

type hardlinkOverlay struct {
	hl  *hardlink
	tgt direntry
}

func (c *hardlinkOverlay) header() *Section {
	return c.hl.header()
}

func (c *hardlinkOverlay) open(r io.ReaderAt, path string) openDirentry {
	switch x := c.tgt.(type) {
	case *file:
		open := x.open(r, path).(*openFile)
		open.fileInfo = c.hl.header()
		return open
	case *dir:
		open := x.open(r, path).(*openDir)
		open.fileInfo = c.hl.header()
		return open
	}
	panic(fmt.Errorf("hardlink targetting unknown type (%T)", c.tgt))
}

func (c *hardlinkOverlay) readLink() (string, error) {
	return "", pathErr("readlink", "", syscall.EINVAL)
}
