package overlayfs

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// lookup is the merged single-path lookup with NO symlink following. full is a
// clean top-relative path (already joined with f.base). It returns the winning
// layer's fsys together with the [fs.FileInfo] from that layer, applying
// whiteout/opaque masking via the ovl-node table.
//
// Resolution order (PLAN §5.1):
//  1. whiteout on full or any ancestor → fs.ErrNotExist.
//  2. top.Lstat(full) → top wins if present.
//  3. opaque on a strict ancestor → lowers hidden → fs.ErrNotExist.
//  4. lowerLstat(full) → highest-priority lower, or fs.ErrNotExist.
//
// The root "." always exists and is answered from top.
func (f *Fs) lookup(full string) (fsys vroot.Fs[vroot.File], info fs.FileInfo, err error) {
	if full == "." {
		info, err = f.top.Lstat(".")
		if err != nil {
			return nil, nil, err
		}
		return f.top, info, nil
	}

	if f.nodes.ancestorWhiteout(full) {
		return nil, nil, fs.ErrNotExist
	}

	info, err = f.top.Lstat(full)
	if err == nil {
		return f.top, info, nil
	}
	if !isNotExist(err) {
		return nil, nil, err
	}

	if f.nodes.ancestorOpaque(full) {
		return nil, nil, fs.ErrNotExist
	}

	return lowerLstat(f.lowers, full)
}

// resolve cleans name, resolves symlinks across the MERGED view, confines the
// result to f.base, and returns the full top-relative path. followLast controls
// whether the final component's symlink is followed (Stat/Open) or left as-is
// (Lstat/ReadLink).
//
// Errors are returned raw (vroot.ErrPathEscapes / fs.ErrNotExist / fs.ErrInvalid);
// the public methods wrap them with the ORIGINAL name.
func (f *Fs) resolve(name string, followLast bool) (full string, err error) {
	cleaned := path.Clean(filepath.ToSlash(name))
	if cleaned == "" || name == "" {
		return "", fs.ErrInvalid
	}

	// ResolvePath operates in the SUB namespace (base-relative) via mergedResolver;
	// skipLastElement == !followLast leaves the final component unresolved for
	// Lstat/ReadLink.
	resolved, err := fsutil.ResolvePath(mergedResolver{f}, name, !followLast)
	if err != nil {
		return "", err
	}

	if f.base == "" {
		return resolved, nil
	}
	if resolved == "." {
		return f.base, nil
	}
	return path.Join(f.base, resolved), nil
}

// mergedResolver adapts a *Fs to the fsutil.LstatFs + fsutil.ReadLinkFs pair
// ResolvePath needs, operating in the sub (base-relative) namespace. Its Lstat
// and ReadLink join f.base before consulting the merged view, so confinement to
// base is automatic; fs.ErrNotExist is propagated unchanged so ResolvePath can
// report it.
type mergedResolver struct {
	f *Fs
}

var (
	_ fsutil.LstatFs    = mergedResolver{}
	_ fsutil.ReadLinkFs = mergedResolver{}
)

// Lstat returns the merged-view FileInfo of the sub-relative path p (no symlink
// following).
func (m mergedResolver) Lstat(p string) (fs.FileInfo, error) {
	full := m.full(p)
	_, info, err := m.f.lookup(full)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// ReadLink returns the symlink target of the sub-relative path p, read from the
// winning layer in the merged view.
func (m mergedResolver) ReadLink(p string) (string, error) {
	full := m.full(p)
	winFsys, _, err := m.f.lookup(full)
	if err != nil {
		return "", err
	}
	return winFsys.ReadLink(full)
}

// full joins f.base with the sub-relative path p (cleaned to slash form).
func (m mergedResolver) full(p string) string {
	cp := path.Clean(filepath.ToSlash(p))
	if m.f.base == "" {
		return cp
	}
	if cp == "." {
		return m.f.base
	}
	return path.Join(m.f.base, cp)
}

// isNotExist reports whether err means "does not exist" in any of the forms the
// layers may produce (fs.ErrNotExist, syscall.ENOENT via *fs.PathError, …).
func isNotExist(err error) bool {
	return err != nil && errors.Is(err, fs.ErrNotExist)
}
