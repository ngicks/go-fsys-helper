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
	r        io.ReaderAt
	root     *dir
	subRoot  *dir
	isRooted bool // If true, prevents path escapes beyond root/subRoot
}

type FsOption struct {
	// If true, tar.TypeChar, tar.TypeBlock, tar.TypeFifo are added as a file.
	AllowDev bool
	// If true, tar.Type
	HandleSymlink bool
	// If true, Fs and Sub-Fs disallow traversing upward from its root via symlink.
	// Hardlinks are still resolved
	IsRooted bool
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
		r:        r,
		isRooted: opt.IsRooted,
	}

	if rootHeader, ok := headers["."]; ok {
		fsys.subRoot = &dir{h: rootHeader}
	} else {
		// Is it even possible?
		fsys.subRoot = &dir{
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
		fsys.subRoot.addChild(key, headers[key])
	}

	// for root fsys root and subRoot is totally same.
	// But different for sub-fsys returned from Sub.
	fsys.root = fsys.subRoot

	return fsys, nil
}

func (fsys *Fs) resolveHardlink(hl *hardlink) (direntry, error) {
	target := path.Clean(hl.header().Header().Linkname)
	return fsys.root.openChild(target, false, fsys)
}

func (fsys *Fs) findFile(name string, skipLastElement bool) (direntry, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}

	// Use our internal symlink resolution
	dirent, err := fsys.subRoot.openChild(name, skipLastElement, fsys)
	if err != nil {
		overrideErr(err, func(err *fs.PathError) { err.Path = name })
		return nil, err
	}

	// Handle hardlinks
	if hl, ok := dirent.(*hardlink); ok {
		target, err := fsys.resolveHardlink(hl)
		if err != nil {
			return nil, err
		}
		return hl.overlayHardlink(target), nil
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

var _ fs.SubFS = (*Fs)(nil)

func (fsys *Fs) Sub(dirPath string) (fs.FS, error) {
	if !fs.ValidPath(dirPath) {
		return nil, &fs.PathError{Op: "sub", Path: dirPath, Err: fs.ErrInvalid}
	}

	f, err := fsys.findFile(dirPath, false)
	if err != nil {
		return nil, &fs.PathError{Op: "sub", Path: dirPath, Err: err}
	}

	subRootDir, ok := f.(*dir)
	if !ok {
		return nil, &fs.PathError{Op: "sub", Path: dirPath, Err: fs.ErrNotExist}
	}

	return &Fs{
		r:        fsys.r,
		isRooted: fsys.isRooted,
		root:     fsys.root,
		subRoot:  subRootDir,
	}, nil
}
