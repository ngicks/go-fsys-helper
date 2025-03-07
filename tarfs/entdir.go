package tarfs

import (
	"archive/tar"
	"io"
	"io/fs"
	"strings"
	"sync"
	"syscall"
)

type dir struct {
	h       *header
	files   map[string]direntry
	ordered []direntry
}

func (d *dir) header() *header {
	return d.h
}

func (d *dir) open(_ io.ReaderAt, path string) openDirentry {
	return &openDir{path: path, dir: d}
}

func (d *dir) addChild(name string, hdr *header) {
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
