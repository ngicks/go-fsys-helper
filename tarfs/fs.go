package tarfs

import (
	"archive/tar"
	"io"
	"io/fs"
	"maps"
	"slices"
)

var _ fs.FS = (*Fs)(nil)

type Fs struct {
	r    io.ReaderAt
	root *dir
}

func New(r io.ReaderAt) (*Fs, error) {
	headers, err := collectHeaders(r)
	if err != nil {
		return nil, err
	}

	fsys := &Fs{
		r:    r,
		root: &dir{},
	}

	if header, ok := headers["."]; ok {
		fsys.root = &dir{h: header}
	}
	delete(headers, ".")

	for _, key := range slices.Sorted(maps.Keys(headers)) {
		switch headers[key].h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
		case tar.TypeDir:
		default:
			continue
		}
		fsys.root.addChild(key, headers[key])
	}

	return fsys, nil
}

func (fsys *Fs) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	f, err := fsys.root.openChild(name)
	if err != nil {
		overrideErr(err, func(err *fs.PathError) { err.Path = name })
		return nil, err
	}
	return f.open(fsys.r, name), err
}
