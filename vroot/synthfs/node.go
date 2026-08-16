package synthfs

import (
	"container/list"
	"io/fs"
	"time"
)

// nodeKind identifies the concrete node type so callers can branch without a
// type switch when the concrete type is irrelevant.
type nodeKind int

const (
	kindDir nodeKind = iota
	kindFile
	kindSymlink
)

// node is the in-tree representation of a directory entry.
//
// All metadata reads and writes (mode, modTime, uid/gid, refCount, and a dir's
// entries) happen under [state.mu]. A *file's content is owned by its
// [FileView]; the view supplies its own synchronization for byte-level I/O.
type node interface {
	meta() *metadata
	kind() nodeKind
}

// metadata are the per-node fields shared by dir/file/symlink.
type metadata struct {
	name     string
	mode     fs.FileMode
	modTime  time.Time
	size     int64 // meaningful for files only; for dirs/symlinks left at 0
	uid, gid int
	refCount int // open handle count; consulted by DisableOpenFileRemoval
}

func (m *metadata) toStat() stat {
	return stat{
		name:    m.name,
		size:    m.size,
		mode:    m.mode,
		modTime: m.modTime,
	}
}

type dir struct {
	metadata
	parent *dir
	// ordered preserves insertion order for ReadDir; index gives O(1) lookup
	// by basename. Both hold the same *list.Element values pointing to nodes.
	ordered *list.List
	index   map[string]*list.Element
}

type file struct {
	metadata
	view FileView
}

type symlink struct {
	metadata
	target string // forward-slash form; may be relative or absolute
}

func (d *dir) meta() *metadata     { return &d.metadata }
func (d *dir) kind() nodeKind      { return kindDir }
func (f *file) meta() *metadata    { return &f.metadata }
func (f *file) kind() nodeKind     { return kindFile }
func (s *symlink) meta() *metadata { return &s.metadata }
func (s *symlink) kind() nodeKind  { return kindSymlink }

func newDir(name string, mode fs.FileMode, modTime time.Time, parent *dir) *dir {
	return &dir{
		metadata: metadata{
			name:    name,
			mode:    fs.ModeDir | (mode & fs.ModePerm),
			modTime: modTime,
		},
		parent:  parent,
		ordered: list.New(),
		index:   make(map[string]*list.Element),
	}
}

// lookup returns the child entry stored under name, or nil if absent.
// Caller must hold state.mu (read lock is sufficient).
func (d *dir) lookup(name string) node {
	e, ok := d.index[name]
	if !ok {
		return nil
	}
	return e.Value.(node)
}

// addEntry inserts n under name, replacing any existing entry with the same
// name. Caller must hold state.mu (write lock).
func (d *dir) addEntry(name string, n node) {
	if old, ok := d.index[name]; ok {
		d.ordered.Remove(old)
		delete(d.index, name)
	}
	e := d.ordered.PushBack(n)
	d.index[name] = e
}

// removeEntry unlinks the entry stored under name and returns it. Returns nil
// if no such entry. Caller must hold state.mu (write lock).
func (d *dir) removeEntry(name string) node {
	e, ok := d.index[name]
	if !ok {
		return nil
	}
	d.ordered.Remove(e)
	delete(d.index, name)
	return e.Value.(node)
}

// entriesSnapshot returns the entries in insertion order. Caller must hold
// state.mu (read lock).
func (d *dir) entriesSnapshot() []node {
	out := make([]node, 0, d.ordered.Len())
	for e := d.ordered.Front(); e != nil; e = e.Next() {
		out = append(out, e.Value.(node))
	}
	return out
}
