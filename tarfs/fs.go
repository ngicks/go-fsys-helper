package tarfs

import (
	"archive/tar"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"
)

var _ fs.FS = (*Fs)(nil)

type Fs struct {
	r    io.ReaderAt
	root *dir
}

type FsOption struct {
	// If true, tar.TypeChar, tar.TypeBlock, tar.TypeFifo are added as a file.
	AllowDev      bool
	handleSymlink bool // export this field when symlink support is added.
}

func New(r io.ReaderAt, opt *FsOption) (*Fs, error) {
	if opt == nil {
		opt = new(FsOption)
	}
	// first collect entries in the map
	// Tar archives may have duplicate entry for same name for incremental update, etc.
	headers, err := tryCollectHeaderOffsets(Sections(r))
	if err != nil {
		return nil, err
	}

	fsys := &Fs{
		r: r,
	}

	if rootHeader, ok := headers["."]; ok {
		fsys.root = &dir{h: rootHeader}
	} else {
		// Is it even possible?
		fsys.root = &dir{
			h: &Section{
				h: &tar.Header{
					Typeflag: tar.TypeDir,
					Name:     "./",
					Mode:     0o755,
					ModTime:  time.Now(),
				},
			},
		}
	}
	delete(headers, ".")

	for _, key := range slices.Sorted(maps.Keys(headers)) {
		if strings.HasPrefix(key, "..") {
			// reject paths traversing upward even when tarinsecurepath = 1.
			// Anyway fs.ValidPath check will reject this.
			continue
		}
		switch headers[key].h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
		case tar.TypeDir:
		case tar.TypeSymlink:
			if !opt.handleSymlink {
				continue
			}
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			if !opt.AllowDev {
				continue
			}
		default:
			continue
		}
		fsys.root.addChild(key, headers[key])
	}

	return fsys, nil
}

func (fsys *Fs) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	f, err := fsys.root.openChild(name)
	if err != nil {
		overrideErr(err, func(err *fs.PathError) { err.Path = name })
		return nil, err
	}
	return f.open(fsys.r, name), err
}

// TODO: add sub
