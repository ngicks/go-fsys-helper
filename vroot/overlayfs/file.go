package overlayfs

import (
	"fmt"
	"io"
	"io/fs"
	"slices"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// decShim wraps the winning layer's [vroot.File] for a REGULAR-file open. Every
// method forwards to inner; Close calls inner.Close once (guarding
// double-close), then drops the path's open-handle count and offers the node
// for GC.
type decShim struct {
	inner vroot.File
	nodes *ovlNodes
	path  string

	mu     sync.Mutex
	closed bool
}

var _ vroot.File = (*decShim)(nil)

func (s *decShim) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.inner.Close()
	s.nodes.decHandle(s.path)
	s.nodes.gc(s.path)
	return err
}

func (s *decShim) Name() string                            { return s.inner.Name() }
func (s *decShim) Fd() uintptr                             { return s.inner.Fd() }
func (s *decShim) Read(p []byte) (int, error)              { return s.inner.Read(p) }
func (s *decShim) ReadAt(p []byte, off int64) (int, error) { return s.inner.ReadAt(p, off) }
func (s *decShim) Write(p []byte) (int, error)             { return s.inner.Write(p) }
func (s *decShim) WriteAt(p []byte, off int64) (int, error) {
	return s.inner.WriteAt(p, off)
}
func (s *decShim) WriteString(str string) (int, error) { return s.inner.WriteString(str) }
func (s *decShim) Truncate(size int64) error           { return s.inner.Truncate(size) }
func (s *decShim) Seek(off int64, whence int) (int64, error) {
	return s.inner.Seek(off, whence)
}
func (s *decShim) Sync() error                  { return s.inner.Sync() }
func (s *decShim) Stat() (fs.FileInfo, error)   { return s.inner.Stat() }
func (s *decShim) Chmod(mode fs.FileMode) error { return s.inner.Chmod(mode) }
func (s *decShim) Chown(uid, gid int) error     { return s.inner.Chown(uid, gid) }
func (s *decShim) ReadDir(n int) ([]fs.DirEntry, error) {
	return s.inner.ReadDir(n)
}
func (s *decShim) Readdir(n int) ([]fs.FileInfo, error) {
	return s.inner.Readdir(n)
}
func (s *decShim) Readdirnames(n int) ([]string, error) {
	return s.inner.Readdirnames(n)
}

// mergedDir is the [vroot.File] returned by Open on a directory. It serves a
// MERGED listing of top ∪ lowers with whiteout/opaque masking applied. The
// listing is computed lazily on first read and snapshotted (lowers are
// immutable; top is snapshotted at first read — mirroring the old overlayFile's
// static assumption). It anchors the dir's ovl-node (handle accounting).
type mergedDir struct {
	f       *Fs
	dirFull string // clean top-relative path of the directory
	name    string // user-facing path (passed to Open)

	mu      sync.Mutex
	closed  bool
	cursor  int
	loaded  bool
	entries []fs.DirEntry
}

var _ vroot.File = (*mergedDir)(nil)

func (d *mergedDir) Name() string { return d.name }
func (d *mergedDir) Fd() uintptr  { return ^uintptr(0) }
func (d *mergedDir) Sync() error  { return nil }

func (d *mergedDir) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()
	d.f.nodes.decHandle(d.dirFull)
	d.f.nodes.gc(d.dirFull)
	return nil
}

func (d *mergedDir) Stat() (fs.FileInfo, error) {
	_, info, err := d.f.lookup(d.dirFull)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", d.name, err)
	}
	return info, nil
}

// load computes the merged dir listing once and caches it. Caller holds d.mu.
//
// Algorithm (PLAN §5.4):
//   - top entries win; a top entry whited out is skipped (defensive).
//   - if dirFull is not opaque, lower entries fill names not already present,
//     skipping whited-out names.
//   - the result is sorted by name.
func (d *mergedDir) load() error {
	if d.loaded {
		return nil
	}
	result := make(map[string]fs.DirEntry)

	topEntries, err := vroot.ReadDir(d.f.top, d.dirFull)
	if err != nil && !isNotExist(err) {
		return err
	}
	for _, e := range topEntries {
		child := joinChild(d.dirFull, e.Name())
		if d.f.nodes.ancestorWhiteout(child) {
			continue
		}
		result[e.Name()] = e
	}

	if !d.f.nodes.opaqueAt(d.dirFull) {
		lowerEntries, err := lowerReadDir(d.f.lowers, d.dirFull)
		if err != nil {
			return err
		}
		for name, e := range lowerEntries {
			if _, ok := result[name]; ok {
				continue
			}
			child := joinChild(d.dirFull, name)
			if d.f.nodes.ancestorWhiteout(child) {
				continue
			}
			result[name] = e
		}
	}

	entries := make([]fs.DirEntry, 0, len(result))
	for _, e := range result {
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return cmpString(a.Name(), b.Name())
	})
	d.entries = entries
	d.loaded = true
	return nil
}

// take advances the cursor by up to n entries; n<=0 returns all remaining (and
// never io.EOF), n>0 returns up to n and io.EOF when already exhausted. Mirrors
// synthfs's dirHandle.readEntries semantics. Caller holds d.mu.
func (d *mergedDir) take(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fsutil.WrapPathErr("readdir", d.name, fs.ErrClosed)
	}
	if err := d.load(); err != nil {
		return nil, fsutil.WrapPathErr("readdir", d.name, err)
	}
	if d.cursor >= len(d.entries) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	end := len(d.entries)
	if n > 0 && d.cursor+n < end {
		end = d.cursor + n
	}
	out := make([]fs.DirEntry, end-d.cursor)
	copy(out, d.entries[d.cursor:end])
	d.cursor = end
	return out, nil
}

func (d *mergedDir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.take(n)
}

func (d *mergedDir) Readdir(n int) ([]fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := d.take(n)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(entries))
	for _, de := range entries {
		info, err := de.Info()
		if err != nil {
			return nil, fsutil.WrapPathErr("readdir", d.name, err)
		}
		out = append(out, info)
	}
	return out, nil
}

func (d *mergedDir) Readdirnames(n int) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := d.take(n)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, de := range entries {
		out = append(out, de.Name())
	}
	return out, nil
}

// Seek mirrors synthfs's dir-handle Seek: only no-op positioning and rewinds to
// start / jumps to end are accepted.
func (d *mergedDir) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, fsutil.WrapPathErr(
				"seek", d.name,
				fmt.Errorf("negative offset %d: %w", offset, fs.ErrInvalid),
			)
		}
		d.cursor = 0
	case io.SeekCurrent:
		if offset != 0 {
			return 0, fsutil.WrapPathErr("seek", d.name, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, fsutil.WrapPathErr(
				"seek", d.name,
				fmt.Errorf("positive offset %d: %w", offset, fs.ErrInvalid),
			)
		}
		if err := d.load(); err != nil {
			return 0, fsutil.WrapPathErr("seek", d.name, err)
		}
		d.cursor = len(d.entries)
	default:
		return 0, fsutil.WrapPathErr(
			"seek", d.name,
			fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid),
		)
	}
	return 0, nil
}

func (d *mergedDir) Read([]byte) (int, error) {
	return 0, fsutil.WrapPathErr("read", d.name, syscall.EISDIR)
}

func (d *mergedDir) ReadAt([]byte, int64) (int, error) {
	return 0, fsutil.WrapPathErr("readat", d.name, syscall.EISDIR)
}

func (d *mergedDir) Write([]byte) (int, error) {
	return 0, fsutil.WrapPathErr("write", d.name, syscall.EISDIR)
}

func (d *mergedDir) WriteAt([]byte, int64) (int, error) {
	return 0, fsutil.WrapPathErr("writeat", d.name, syscall.EISDIR)
}

func (d *mergedDir) WriteString(string) (int, error) {
	return 0, fsutil.WrapPathErr("write", d.name, syscall.EISDIR)
}

func (d *mergedDir) Truncate(int64) error {
	return fsutil.WrapPathErr("truncate", d.name, syscall.EISDIR)
}

// Chmod routes to top if the directory is present there. For read-path 3a this
// keeps a top-backed dir Chmod working; a lower-only dir Chmod is deferred to
// the write path (Wave 3b) and reports a read-only error.
func (d *mergedDir) Chmod(mode fs.FileMode) error {
	if _, err := d.f.top.Lstat(d.dirFull); err == nil {
		return fsutil.WrapPathErr("chmod", d.name, d.f.top.Chmod(d.dirFull, mode))
	}
	return fsutil.WrapPathErr("chmod", d.name, errdef.EROFS)
}

// Chown routes to top if the directory is present there; otherwise reports a
// read-only error (Wave 3b handles copy-up).
func (d *mergedDir) Chown(uid, gid int) error {
	if _, err := d.f.top.Lstat(d.dirFull); err == nil {
		return fsutil.WrapPathErr("chown", d.name, d.f.top.Chown(d.dirFull, uid, gid))
	}
	return fsutil.WrapPathErr("chown", d.name, errdef.EROFS)
}

// joinChild joins a directory path with a child name, yielding a clean
// top-relative path. For the root dir (".") the child stands alone.
func joinChild(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// cmpString is a tiny three-way string comparison used for sorting dir entries.
func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
