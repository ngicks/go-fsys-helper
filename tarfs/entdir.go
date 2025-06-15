package tarfs

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"syscall"
)

type dir struct {
	h       *Section
	files   map[string]direntry
	ordered []direntry
}

func (d *dir) header() *Section {
	return d.h
}

func (d *dir) open(_ io.ReaderAt, path string) openDirentry {
	return &openDir{path: path, dir: d}
}

func (d *dir) addChild(name string, hdr *Section) {
	if d.files == nil {
		d.files = make(map[string]direntry)
	}
	name, rest, ok := strings.Cut(name, "/")
	if ok {
		child, ok := d.files[name]
		if !ok {
			child = &dir{}
			d.files[name] = child
			d.ordered = append(d.ordered, child)
		}
		child.(*dir).addChild(rest, hdr)
	} else {
		var ent direntry
		if hdr.h.Typeflag == tar.TypeDir {
			if existiing := d.files[name]; existiing != nil {
				existiing.(*dir).h = hdr
			} else {
				ent = &dir{h: hdr}
			}
		} else {
			// TODO: handle symlink after Go1.25
			ent = &file{h: hdr}
		}
		if ent != nil {
			d.files[name] = ent
			d.ordered = append(d.ordered, ent)
		}
	}
}

func (d *dir) openChild(name string) (direntry, error) {
	if name == "." {
		return d, nil
	}

	name, rest, ok := strings.Cut(name, "/")
	child := d.files[name]
	if child == nil {
		return nil, pathErr("open", name, fs.ErrNotExist)
	}
	if ok {
		switch x := child.(type) {
		case *dir:
			return x.openChild(rest)
			// TODO: handle symlink after Go1.25. tarfs should be always rooted.
		default:
			return nil, pathErr("open", name, syscall.ENOTDIR)
		}
	}
	return child, nil
}

var (
	_ fs.File        = (*openDir)(nil)
	_ fs.ReadDirFile = (*openDir)(nil)
)

type openDir struct {
	mu     sync.Mutex
	closed bool

	cursor int

	dir  *dir
	path string
}

func (d *openDir) checkClosed(op string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return pathErr(op, d.path, fs.ErrClosed)
	}
	return nil
}

func (d *openDir) Name() string {
	return d.path
}

func (d *openDir) Stat() (fs.FileInfo, error) {
	if err := d.checkClosed("stat"); err != nil {
		return nil, err
	}
	return d.dir.h.h.FileInfo(), nil
}

func (d *openDir) Read([]byte) (int, error) {
	if err := d.checkClosed("read"); err != nil {
		return 0, err
	}
	return 0, pathErr("read", d.path, syscall.EISDIR)
}

func (d *openDir) ReadAt(p []byte, off int64) (n int, err error) {
	if err := d.checkClosed("readat"); err != nil {
		return 0, err
	}
	return 0, pathErr("read", d.path, syscall.EISDIR)
}

func (d *openDir) Seek(offset int64, whence int) (int64, error) {
	if err := d.checkClosed("seek"); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	// mimicking *os.File behavior, reset anyway unless io.SeekEnd and 0 is set.
	d.cursor = 0

	// lseek(3) on directory is not totally defined as far as I know.
	//
	// https://man7.org/linux/man-pages/man2/lseek.2.html
	// https://stackoverflow.com/questions/65911066/what-does-lseek-mean-for-a-directory-file-descriptor
	//
	// on windows Seek calls SetFilePointerEx. Does it work on handle for folder? I'm zero sure.
	//
	// So here place a simple rule.

	switch whence {
	default:
		return 0, pathErr("seek", d.path, fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid))
	case io.SeekStart:
		if offset < 0 {
			return 0, pathErr("seek", d.path, fmt.Errorf("negative offset %d: %w", whence, fs.ErrInvalid))
		}
	case io.SeekCurrent:
		if offset != 0 {
			return 0, pathErr("seek", d.path, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, pathErr("seek", d.path, fmt.Errorf("positive offset %d: %w", whence, fs.ErrInvalid))
		}
		d.cursor = len(d.dir.ordered)
	}
	return 0, nil
}

func (d *openDir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	// double close is fine for this.
	d.closed = true
	return nil
}

func (d *openDir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, pathErr("readdir", d.path, fs.ErrClosed)
	}

	if d.cursor >= len(d.dir.files) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		n = len(d.dir.ordered) - d.cursor
	}

	out := make([]fs.DirEntry, min(n, len(d.dir.files)-d.cursor))
	for i := range out {
		out[i] = fs.FileInfoToDirEntry(d.dir.ordered[d.cursor].header().h.FileInfo())
		d.cursor++
	}

	return out, nil
}
