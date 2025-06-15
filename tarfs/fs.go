package tarfs

import (
	"archive/tar"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"time"
)

var _ fs.FS = (*Fs)(nil)

// Compile-time check for fs.ReadLinkFS interface (Go 1.25+)
// This will be available when Go 1.25 is released
// var _ fs.ReadLinkFS = (*Fs)(nil)

type Fs struct {
	r    io.ReaderAt
	root *dir
}

type FsOption struct {
	// If true, tar.TypeChar, tar.TypeBlock, tar.TypeFifo are added as a file.
	AllowDev bool
	// If true, tar.Type
	HandleSymlink bool
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
			if !opt.HandleSymlink {
				continue
			}
		case tar.TypeLink:
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

func (fsys *Fs) open(name string) (direntry, error) {
	// TODO: unroll openChild's recursive way to mere for loop.
	return fsys.root.openChild(name)
}

func (fsys *Fs) openResolve(name string) (direntry, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}

	dirent, err := fsys.open(name)
	if err != nil {
		return nil, err
	}

	hl, ok := dirent.(*hardlink)
	if !ok {
		return dirent, nil
	}

	dirent, err = fsys.open(path.Clean(hl.header().Header().Linkname))
	if err != nil {
		return nil, err
	}

	return hl.overlayHardlink(dirent), nil
}

func (fsys *Fs) findFile(name string, skipLastElement bool) (direntry, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}

	resolved, err := fsys.resolvePath(name, skipLastElement)
	if err != nil {
		overrideErr(err, func(err *fs.PathError) { err.Path = name })
		return nil, err
	}

	dirent, err := fsys.openResolve(resolved)
	if err != nil {
		overrideErr(err, func(err *fs.PathError) { err.Path = name })
		return nil, err
	}

	return dirent, nil
}

func (fsys *Fs) Open(name string) (fs.File, error) {
	f, err := fsys.findFile(name, false)
	if err != nil {
		return nil, err
	}
	return f.open(fsys.r, name), nil
}

func (fsys *Fs) ReadLink(name string) (string, error) {
	f, err := fsys.findFile(name, true)
	if err != nil {
		return "", err
	}
	return f.readLink()
}

func (fsys *Fs) Lstat(name string) (fs.FileInfo, error) {
	f, err := fsys.findFile(name, true)
	if err != nil {
		return nil, err
	}
	return f.header().Header().FileInfo(), nil
}
