package synthfs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Root[vroot.File, *Root] = (*Root)(nil)

// Root is the synthetic filesystem. Multiple Roots derived from one another
// via [Root.OpenRoot] share the same in-memory tree; each Root only differs in
// which dir it treats as its confinement boundary.
//
// The zero value is invalid; use [NewRoot].
type Root struct {
	st       *state
	boundary *dir   // confinement boundary — ".." past here is rejected
	name     string // returned by Name(); set at construction
}

// NewRoot constructs an empty synthetic filesystem. name is reported by
// [Root.Name]; opt nil yields all defaults (in-memory allocator, 0o022 umask,
// real wall clock, simple chmod mask, ERROR_SHARING_VIOLATION off).
func NewRoot(name string, opt *Option) *Root {
	st := &state{opt: opt.resolve()}
	st.root = newDir("", fs.FileMode(0o777)&^st.opt.umask, st.opt.clock.Now(), nil)
	return &Root{st: st, boundary: st.root, name: name}
}

// IsRoot is the [vroot.Root] marker.
func (r *Root) IsRoot() {}

// Name returns the name passed to [NewRoot] (or, for a sub-root opened via
// [Root.OpenRoot], the parent's name joined with the sub-path).
func (r *Root) Name() string {
	return r.name
}

// Close is a no-op; an in-memory tree owns no OS handles.
func (r *Root) Close() error {
	return nil
}

// OpenRoot returns a new [Root] whose confinement boundary is the directory
// at name. The new Root shares the underlying tree, so changes through one
// are visible through the other.
func (r *Root) OpenRoot(name string) (*Root, error) {
	r.st.mu.RLock()
	res, err := r.findNode(name, true, "open")
	if err != nil {
		r.st.mu.RUnlock()
		return nil, err
	}
	d, ok := res.node.(*dir)
	if !ok {
		r.st.mu.RUnlock()
		return nil, toPathErr("openroot", name, syscall.ENOTDIR)
	}
	r.st.mu.RUnlock()
	return &Root{
		st:       r.st,
		boundary: d,
		name:     path.Join(r.name, filepath.ToSlash(name)),
	}, nil
}

// findNode resolves name under r's confinement boundary.
func (r *Root) findNode(name string, followLast bool, op string) (resolveResult, error) {
	return r.st.resolve(r.boundary, r.boundary, name, followLast, op)
}

// withWriteLock runs f under the state write lock.
func (r *Root) withWriteLock(f func() error) error {
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	return f()
}

// userPath maps a name supplied to a Root method back to an OS-form path
// suitable for Name() on the returned file handle.
func (r *Root) userPath(name string) string {
	return filepath.FromSlash(filepath.Clean(name))
}

// toPathErr is a one-liner around [fsutil.WrapPathErr] used by every method
// here; kept as a package-local helper so the imports list stays short.
func toPathErr(op, name string, err error) error {
	return fsutil.WrapPathErr(op, name, err)
}

// endsWithDot mirrors os.RemoveAll's leading check: a trailing "." or "/."
// must return EINVAL before any filepath.Clean rewrites it.
func endsWithDot(name string) bool {
	if name == "." {
		return true
	}
	if len(name) >= 2 && name[len(name)-1] == '.' && os.IsPathSeparator(name[len(name)-2]) {
		return true
	}
	return false
}

// errInternalNodeType is returned when path resolution lands on a node type
// the rest of the package doesn't understand — a bug, not a user error.
var errInternalNodeType = errors.New("synthfs: unknown node type")
