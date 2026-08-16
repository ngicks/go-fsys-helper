package overlayfs

import (
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var (
	_ vroot.File = (*trackedFile)(nil)
	_ vroot.File = (*mergedDir)(nil)
)

// handleTracker counts the handles the overlay has out on each merged-view name,
// which is what [Option.DisableOpenFileRemoval] refuses a removal by.
//
// It locks for itself rather than riding the state lock: opens run under the
// read lock, which lets two of them into the map at once. Counting up still
// happens inside the open's read-locked stretch, so a removal holding the write
// lock sees either an open that finished or one that has not started.
type handleTracker struct {
	mu   sync.Mutex
	open map[string]int
}

func newHandleTracker() *handleTracker {
	return &handleTracker{open: make(map[string]int)}
}

func (t *handleTracker) acquire(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open[name]++
}

// release drops one handle on name. The entry goes with the last of them so the
// map holds only names something is actually open on.
func (t *handleTracker) release(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n := t.open[name] - 1; n > 0 {
		t.open[name] = n
	} else {
		delete(t.open, name)
	}
}

func (t *handleTracker) count(name string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open[name]
}

// countSubtree counts the handles out on name and on everything under it, which
// is what a removal taking a whole tree down is refused by. It scans the map
// rather than walking the tree: the map holds only names something is open on,
// and the tree the walk would cover spans layers.
func (t *handleTracker) countSubtree(name string) int {
	prefix := name + "/"
	if name == "" {
		prefix = ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var n int
	for open, count := range t.open {
		if open == name || strings.HasPrefix(open, prefix) {
			n += count
		}
	}
	return n
}

// trackedFile is a handle the overlay handed out, counted for as long as it is
// open. Everything but Close and Name is the wrapped handle's own.
type trackedFile struct {
	vroot.File
	name    string
	key     string
	handles *handleTracker
	once    sync.Once
}

// Name reports the name the file was opened by rather than the path it has
// inside its layer, which spells the canonical layout the merged view hides.
func (f *trackedFile) Name() string {
	return f.name
}

// Close closes the handle and drops the count exactly once, whatever the wrapped
// Close reports: a failed close is not a handle the caller can close again.
func (f *trackedFile) Close() error {
	err := f.File.Close()
	f.once.Do(func() { f.handles.release(f.key) })
	return err
}

// mergedDir is the handle a directory opens as: the merged listing of every
// layer contributing to it, read through a cursor, over the handle the layer the
// directory itself resolved to answered with.
//
// The layer handle is kept for what only it can answer — Stat, Fd — and closed
// with this one. It is never read from: its listing is one layer's, and the
// merged one is the point. It moves with the directory: a change through this
// handle copies the directory up, and the handle then has to be the top's.
type mergedDir struct {
	fsys *Fs
	// name is the caller's own spelling, key the merged-view name the listing is
	// read at.
	name string
	key  string

	mu      sync.Mutex
	inner   vroot.File
	closed  bool
	cursor  int
	entries []fs.DirEntry
	loaded  bool
}

func newMergedDir(fsys *Fs, name string, key string, inner vroot.File) *mergedDir {
	return &mergedDir{fsys: fsys, inner: inner, name: name, key: key}
}

func (d *mergedDir) Name() string { return d.name }

func (d *mergedDir) Fd() uintptr {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.inner.Fd()
}

func (d *mergedDir) Stat() (fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.inner.Stat()
}

// Sync reports success without reaching the layer, as synthfs does: a directory
// handle here is a listing and a cursor, and there is nothing of it to flush.
func (d *mergedDir) Sync() error { return nil }

func (d *mergedDir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.inner.Close()
}

// load takes the merged listing once and paginates over that. Recomputing it per
// call would let entries shift between two ReadDir(n) calls, which is exactly
// where a cursor into it stops meaning anything; a [mergedDir.Seek] back to the
// start drops the snapshot and the next read takes a fresh one.
//
// The caller holds d.mu and no overlay lock: this is where the handle reaches
// back into the state, between operations rather than inside one.
func (d *mergedDir) load() error {
	if d.loaded {
		return nil
	}
	d.fsys.state.mu.RLock()
	defer d.fsys.state.mu.RUnlock()
	entries, err := d.fsys.state.readDir(d.key)
	if err != nil {
		return err
	}
	d.entries = entries
	d.loaded = true
	return nil
}

// readEntries advances the cursor by up to n entries; n <= 0 means "the rest".
// An exhausted cursor answers a bounded read with [io.EOF] and an unbounded one
// with nothing, the way [os.File] does.
func (d *mergedDir) readEntries(n int) ([]fs.DirEntry, error) {
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
	out := slices.Clone(d.entries[d.cursor:end])
	d.cursor = end
	return out, nil
}

func (d *mergedDir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readEntries(n)
}

func (d *mergedDir) Readdir(n int) ([]fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := d.readEntries(n)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, len(entries))
	for i, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, fsutil.WrapPathErr("readdir", d.name, err)
		}
		out[i] = info
	}
	return out, nil
}

func (d *mergedDir) Readdirnames(n int) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := d.readEntries(n)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out, nil
}

// Seek mimics [os.File] on a directory: only a rewind to the whole listing's
// start, a jump to its end, or the [io.SeekCurrent] no-op is accepted, and
// none reports a meaningful offset. What the cursor indexes is entries, not
// bytes, so an offset naming anything else is refused rather than rounded.
func (d *mergedDir) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch whence {
	case io.SeekStart:
		if offset != 0 {
			return 0, fsutil.WrapPathErr(
				"seek",
				d.name,
				fmt.Errorf("non-zero offset %d: %w", offset, fs.ErrInvalid),
			)
		}
		d.cursor = 0
		d.entries = nil
		d.loaded = false
	case io.SeekCurrent:
		if offset != 0 {
			return 0, fsutil.WrapPathErr("seek", d.name, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset != 0 {
			return 0, fsutil.WrapPathErr(
				"seek",
				d.name,
				fmt.Errorf("non-zero offset %d: %w", offset, fs.ErrInvalid),
			)
		}
		if err := d.load(); err != nil {
			return 0, fsutil.WrapPathErr("seek", d.name, err)
		}
		d.cursor = len(d.entries)
	default:
		return 0, fsutil.WrapPathErr(
			"seek",
			d.name,
			fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid),
		)
	}
	return 0, nil
}

func (d *mergedDir) isDirErr(op string) error {
	return fsutil.WrapPathErr(op, d.name, syscall.EISDIR)
}

func (d *mergedDir) Read([]byte) (int, error) { return 0, d.isDirErr("read") }

func (d *mergedDir) ReadAt([]byte, int64) (int, error) { return 0, d.isDirErr("readat") }

func (d *mergedDir) Write([]byte) (int, error) { return 0, d.isDirErr("write") }

func (d *mergedDir) WriteAt([]byte, int64) (int, error) { return 0, d.isDirErr("writeat") }

func (d *mergedDir) WriteString(string) (int, error) { return 0, d.isDirErr("write") }

func (d *mergedDir) Truncate(int64) error { return d.isDirErr("truncate") }

// Chmod and Chown go through the same copy-up gate a path-addressed change does,
// against the merged-view name the handle was opened at: the directory may have
// resolved to a lower, and a mode set there would be a write to a layer the
// overlay does not own. The state lock is taken here rather than held across the
// handle's life, the way [mergedDir.load] takes it.

func (d *mergedDir) Chmod(mode fs.FileMode) error {
	return d.mutate("chmod",
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Chmod(path, mode) },
	)
}

func (d *mergedDir) Chown(uid int, gid int) error {
	return d.mutate("chown",
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Chown(path, uid, gid) },
	)
}

// mutate runs op through the copy-up gate and points inner at the top's copy of
// the directory afterwards, that being where the change landed: the handle the
// layer answered the open with describes the directory the copy-up left behind,
// and Stat and Fd would go on reporting it.
//
// A closed handle changes nothing, the way [mergedDir.readEntries] reads
// nothing: the copy-up the change would drag in is a write no handle is entitled
// to any more. A handle whose reopen fails keeps the one it has — a change that
// has already landed is not failed over a handle, and the handle is then no
// staler than it was before this method existed. The state lock is taken under
// d.mu, the order [mergedDir.load] takes them in.
func (d *mergedDir) mutate(
	op string,
	fn func(fsys vroot.Fs[vroot.File], path string) error,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fsutil.WrapPathErr(op, d.name, fs.ErrClosed)
	}
	if err := d.fsys.mutateResolved(d.key, fn); err != nil {
		return fsutil.WrapPathErr(op, d.name, err)
	}
	inner, err := d.fsys.openTopDir(d.key)
	if err != nil {
		return nil
	}
	_ = d.inner.Close()
	d.inner = inner
	return nil
}
