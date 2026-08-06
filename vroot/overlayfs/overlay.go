package overlayfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

// Fs is a union mount: a writable top layer stacked over zero or more read-only
// lower layers. It implements [vroot.Root]. Reads resolve top→lowers (top
// wins) with whiteout/opaque masking; writes (Wave 3b) land in top after
// copy-up.
//
// A *Fs derived via [Fs.OpenRoot] (sub == true) shares the same ovl-node table,
// top, lowers, store and policy with its parent; it differs only by a base path
// prefix that confines it.
type Fs struct {
	nodes  *ovlNodes
	top    vroot.Fs[vroot.File]
	lowers []Layer
	base   string // clean slash sub-prefix; "" for the root overlay
	name   string
	opt    resolvedOption
	policy CopyUpPolicy
	sub    bool // true for OpenRoot-derived sub-overlays
}

var _ vroot.Root[vroot.File, *Fs] = (*Fs)(nil)

// Option configures an overlay built by [New]. The zero value is usable; nil is
// equivalent to [DefaultOption].
type Option struct {
	// CopyUpPolicy selects the copy-up strategy. nil → NewCopyUpPolicyStage("").
	CopyUpPolicy CopyUpPolicy
	// DisableOpenFileRemoval rejects Remove/Rename of a path with live open
	// handles (Windows ERROR_SHARING_VIOLATION emulation). Consulted by the
	// write path (Wave 3b).
	DisableOpenFileRemoval bool
}

// resolvedOption is the validated, non-nil-pointer form of [Option].
type resolvedOption struct {
	disableOpenFileRemoval bool
}

// DefaultOption returns an Option with every field at its documented default.
func DefaultOption() *Option {
	return &Option{
		CopyUpPolicy:           NewCopyUpPolicyStage(""),
		DisableOpenFileRemoval: false,
	}
}

// New builds a union mount over a writable top and read-only lowers (ordered
// low→high priority). opt nil ⇒ [DefaultOption].
//
// The overlay's ovl-node table is seeded from top.meta.Load() once. If top.meta
// is nil a [MetadataStoreMem] (ephemeral, no-op persistence) is substituted.
//
// New cannot return an error (PLAN §4 signature). If the seed Load fails the
// overlay starts with an EMPTY node table over the same store — persisted
// whiteouts simply are not seeded — so a usable *Fs is always returned. See
// DECISION.md (D10).
func New(top Layer, lowers []Layer, opt *Option) *Fs {
	if opt == nil {
		opt = DefaultOption()
	}
	policy := opt.CopyUpPolicy
	if policy == nil {
		policy = NewCopyUpPolicyStage("")
	}
	store := top.meta
	if store == nil {
		store = NewMetadataStoreMem()
	}
	nodes, err := newOvlNodes(store)
	if err != nil {
		// Load failed: newOvlNodes still returns a (possibly empty) table over
		// the store. Keep that table; persisted masks are merely unseeded.
		_ = err
	}
	name := top.fsys.Name()
	if name == "" {
		name = "overlayfs"
	}
	return &Fs{
		nodes:  nodes,
		top:    top.fsys,
		lowers: lowers,
		base:   "",
		name:   name,
		opt: resolvedOption{
			disableOpenFileRemoval: opt.DisableOpenFileRemoval,
		},
		policy: policy,
		sub:    false,
	}
}

// IsRoot is the [vroot.Root] marker.
func (f *Fs) IsRoot() {}

// Name returns the overlay's name (the top fsys name, or for a sub-overlay the
// parent name joined with the sub-path).
func (f *Fs) Name() string { return f.name }

// Sync flushes pending metadata-journal writes to durable storage. It is a
// no-op for synchronous/ephemeral stores.
func (f *Fs) Sync() error {
	return f.nodes.store.Flush()
}

// Close flushes and releases the overlay's resources. A sub-overlay shares
// everything with its parent and only no-ops. The root overlay flushes the
// store, closes it if it is an io.Closer, and closes top + every lower,
// gathering errors.
func (f *Fs) Close() error {
	if f.sub {
		return nil
	}
	var errs []error
	if err := f.nodes.store.Flush(); err != nil {
		errs = append(errs, err)
	}
	if c, ok := f.nodes.store.(io.Closer); ok {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := f.top.Close(); err != nil {
		errs = append(errs, err)
	}
	for _, l := range f.lowers {
		if err := l.fsys.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Stat returns the FileInfo of name, following a final-component symlink.
func (f *Fs) Stat(name string) (fs.FileInfo, error) {
	full, err := f.resolve(name, true)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	_, info, err := f.lookup(full)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return info, nil
}

// Lstat returns the FileInfo of name without following a final-component
// symlink.
func (f *Fs) Lstat(name string) (fs.FileInfo, error) {
	full, err := f.resolve(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	_, info, err := f.lookup(full)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return info, nil
}

// ReadLink returns the target of the symlink at name.
func (f *Fs) ReadLink(name string) (string, error) {
	full, err := f.resolve(name, false)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	winFsys, info, err := f.lookup(full)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return "", fsutil.WrapPathErr("readlink", name, errdef.EBADF)
	}
	target, err := winFsys.ReadLink(full)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	return target, nil
}

// Open opens name read-only.
func (f *Fs) Open(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile opens name. A read-only open returns the winning layer's file (or a
// merged directory handle); a write open copies the entry up to top first (or
// creates it fresh in top under O_CREATE, clearing any whiteout). Every
// successful open anchors the path's ovl-node for handle accounting
// (decision 15).
func (f *Fs) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	write := openflag.WriteOp(flag)
	creating := flag&os.O_CREATE != 0

	var (
		full string
		err  error
	)
	if creating {
		// The named entry may not exist yet; confine without requiring existence
		// or following a final symlink.
		full, err = f.confine(name)
	} else {
		full, err = f.resolve(name, true)
	}
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}

	winFsys, info, lerr := f.lookup(full)
	exists := lerr == nil
	if lerr != nil && !isNotExist(lerr) {
		return nil, fsutil.WrapPathErr("open", name, lerr)
	}

	if !write {
		if !exists {
			return nil, fsutil.WrapPathErr("open", name, fs.ErrNotExist)
		}
		if info.IsDir() {
			f.nodes.incHandle(full)
			return &mergedDir{f: f, dirFull: full, name: f.userPath(name)}, nil
		}
		inner, err := winFsys.OpenFile(full, flag, perm)
		if err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
		f.nodes.incHandle(full)
		return &decShim{inner: inner, nodes: f.nodes, path: full}, nil
	}

	// --- write open ---
	if exists && info.IsDir() {
		return nil, fsutil.WrapPathErr("open", name, syscall.EISDIR)
	}
	if exists {
		if creating && flag&os.O_EXCL != 0 {
			return nil, fsutil.WrapPathErr("open", name, fs.ErrExist)
		}
		if err := f.ensureCopiedUp(full); err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	} else {
		if !creating {
			return nil, fsutil.WrapPathErr("open", name, fs.ErrNotExist)
		}
		if pn, ok := f.nodes.peek(full); ok {
			pn.mu.RLock()
			whited := pn.whiteout
			pn.mu.RUnlock()
			if whited {
				if err := f.nodes.clearWhiteout(full); err != nil {
					return nil, fsutil.WrapPathErr("open", name, err)
				}
			}
		}
		if err := f.ensureParentDirs(full); err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	}
	inner, err := f.top.OpenFile(full, flag, perm)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	f.nodes.incHandle(full)
	return &decShim{inner: inner, nodes: f.nodes, path: full}, nil
}

// OpenRoot returns a sub-overlay confined to name. It shares the parent's
// ovl-node table, top, lowers, store and policy; confinement is the overlay's
// own base-prefix boundary (every method joins base).
func (f *Fs) OpenRoot(name string) (*Fs, error) {
	full, err := f.resolve(name, true)
	if err != nil {
		return nil, fsutil.WrapPathErr("openroot", name, err)
	}
	_, info, err := f.lookup(full)
	if err != nil {
		return nil, fsutil.WrapPathErr("openroot", name, err)
	}
	if !info.IsDir() {
		return nil, fsutil.WrapPathErr("openroot", name, syscall.ENOTDIR)
	}
	return &Fs{
		nodes:  f.nodes,
		top:    f.top,
		lowers: f.lowers,
		base:   full,
		name:   path.Join(f.name, f.userPath(name)),
		opt:    f.opt,
		policy: f.policy,
		sub:    true,
	}, nil
}

// userPath maps a name supplied to a method back to a clean slash path for
// Name() on returned handles / sub-roots.
func (f *Fs) userPath(name string) string {
	return path.Clean(filepath.ToSlash(name))
}

// confine cleans name, rejects escapes (absolute / ".." / leading-"..") WITHOUT
// requiring the path to exist or following symlinks, then joins f.base. Used by
// structural create/remove ops whose target may not yet exist (Mkdir, MkdirAll,
// Create, Symlink, Remove, RemoveAll, Rename). Read/metadata ops use resolve,
// which follows symlinks across the merged view and enforces the same boundary.
func (f *Fs) confine(name string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(name))
	if cleaned == "." || cleaned == "" {
		// "." / "" name to a structural op (Remove/Mkdir/…) is invalid, matching
		// os.RemoveAll(".") → EINVAL.
		return "", syscall.EINVAL
	}
	if !fs.ValidPath(cleaned) {
		return "", vroot.ErrPathEscapes
	}
	if f.base == "" {
		return cleaned, nil
	}
	return path.Join(f.base, cleaned), nil
}

// lowerHas reports whether full is resolvable in some lower (no masking applied).
func (f *Fs) lowerHas(full string) bool {
	_, _, err := lowerLstat(f.lowers, full)
	return err == nil
}

// childWhited reports whether the exact path child carries a whiteout bit on its
// own node. Used by mergedDirEmpty, which must not re-lock a held parent node.
func (f *Fs) childWhited(child string) bool {
	if pn, ok := f.nodes.peek(child); ok {
		pn.mu.RLock()
		w := pn.whiteout
		pn.mu.RUnlock()
		return w
	}
	return false
}

// mergedDirEmpty reports whether the merged directory dir has no visible
// children. includeLowers must be false when dir is opaque or hidden under an
// opaque ancestor (its lower children don't count). The caller may hold dir's
// own node lock; this only ever locks the direct CHILD nodes, never dir's.
func (f *Fs) mergedDirEmpty(dir string, includeLowers bool) (bool, error) {
	topEntries, err := vroot.ReadDir(f.top, dir)
	if err != nil && !isNotExist(err) {
		return false, err
	}
	for _, e := range topEntries {
		if !f.childWhited(joinChild(dir, e.Name())) {
			return false, nil
		}
	}
	if includeLowers {
		lower, err := lowerReadDir(f.lowers, dir)
		if err != nil {
			return false, err
		}
		for name := range lower {
			if !f.childWhited(joinChild(dir, name)) {
				return false, nil
			}
		}
	}
	return true, nil
}

// ---- write path -----------------------------------------------------------
//
// Concurrency model: see DECISION.md D12. Structural single-path ops take the
// target node's write lock and, while holding it, touch ONLY that node's bits
// directly, strict-ancestor masking checks (which lock other nodes), and raw
// top/lower ops — never lookup(full)/ancestorWhiteout(full)/the re-locking
// helpers on full (Go RWMutex is non-reentrant). Whiteouts are flipped with an
// ordering that keeps every lock-free reader's view consistent. Copy-up runs
// via nodes.copyUp (singleflight) with no node lock held.

// Chmod copies the target up (if lower-only) and chmods the top copy. Follows a
// final symlink, matching os.Chmod.
func (f *Fs) Chmod(name string, mode fs.FileMode) error {
	full, err := f.resolve(name, true)
	if err != nil {
		return fsutil.WrapPathErr("chmod", name, err)
	}
	if err := f.ensureCopiedUp(full); err != nil {
		return fsutil.WrapPathErr("chmod", name, err)
	}
	return fsutil.WrapPathErr("chmod", name, f.top.Chmod(full, mode))
}

// Chown copies the target up and chowns the top copy (follows a final symlink).
func (f *Fs) Chown(name string, uid, gid int) error {
	full, err := f.resolve(name, true)
	if err != nil {
		return fsutil.WrapPathErr("chown", name, err)
	}
	if err := f.ensureCopiedUp(full); err != nil {
		return fsutil.WrapPathErr("chown", name, err)
	}
	return fsutil.WrapPathErr("chown", name, f.top.Chown(full, uid, gid))
}

// Chtimes copies the target up and sets times on the top copy (follows a final
// symlink).
func (f *Fs) Chtimes(name string, atime, mtime time.Time) error {
	full, err := f.resolve(name, true)
	if err != nil {
		return fsutil.WrapPathErr("chtimes", name, err)
	}
	if err := f.ensureCopiedUp(full); err != nil {
		return fsutil.WrapPathErr("chtimes", name, err)
	}
	return fsutil.WrapPathErr("chtimes", name, f.top.Chtimes(full, atime, mtime))
}

// Lchown copies the symlink itself up and lchowns it (does not follow a final
// symlink).
func (f *Fs) Lchown(name string, uid, gid int) error {
	full, err := f.resolve(name, false)
	if err != nil {
		return fsutil.WrapPathErr("lchown", name, err)
	}
	if err := f.ensureCopiedUp(full); err != nil {
		return fsutil.WrapPathErr("lchown", name, err)
	}
	return fsutil.WrapPathErr("lchown", name, f.top.Lchown(full, uid, gid))
}

// Create is OpenFile with O_RDWR|O_CREATE|O_TRUNC, perm 0o666.
func (f *Fs) Create(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// Mkdir creates a directory at name in top. Over a cleared whiteout that had
// lower content it sets the node opaque (decision 13) so stale lower children
// stay hidden.
func (f *Fs) Mkdir(name string, perm fs.FileMode) error {
	full, err := f.confine(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	n := f.nodes.get(full)
	n.mu.Lock()
	defer n.mu.Unlock()

	ancWhited := f.nodes.ancestorWhiteout(path.Dir(full))
	if !n.whiteout && !ancWhited {
		if _, err := f.top.Lstat(full); err == nil {
			return fsutil.WrapPathErr("mkdir", name, fs.ErrExist)
		} else if !isNotExist(err) {
			return fsutil.WrapPathErr("mkdir", name, err)
		}
		if !f.nodes.ancestorOpaque(full) && f.lowerHas(full) {
			return fsutil.WrapPathErr("mkdir", name, fs.ErrExist)
		}
	}

	if err := f.ensureParentDirs(full); err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	if err := f.top.Mkdir(full, perm); err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}

	// Order: opaque before clearing whiteout so there is no window in which full
	// is visible without its lower children being masked.
	if f.lowerHas(full) {
		n.opaque = true
		if err := f.nodes.store.SetOpaque(full); err != nil {
			return fsutil.WrapPathErr("mkdir", name, err)
		}
	}
	if n.whiteout {
		n.whiteout = false
		if err := f.nodes.store.ClearWhiteout(full); err != nil {
			return fsutil.WrapPathErr("mkdir", name, err)
		}
	}
	return nil
}

// MkdirAll creates name and any missing parents in top, applying Mkdir's
// opaque/whiteout semantics per created component. Existing directories are
// tolerated; an existing non-directory component is ENOTDIR.
func (f *Fs) MkdirAll(name string, perm fs.FileMode) error {
	full, err := f.confine(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	for _, comp := range pathChain(full) {
		_, info, lerr := f.lookup(comp)
		if lerr == nil {
			if info.IsDir() {
				continue
			}
			return fsutil.WrapPathErr("mkdir", name, syscall.ENOTDIR)
		}
		if !isNotExist(lerr) {
			return fsutil.WrapPathErr("mkdir", name, lerr)
		}
		if err := f.mkdirOne(comp, perm); err != nil {
			return fsutil.WrapPathErr("mkdir", name, err)
		}
	}
	return nil
}

// mkdirOne creates a single component in top with Mkdir's masking semantics.
func (f *Fs) mkdirOne(full string, perm fs.FileMode) error {
	n := f.nodes.get(full)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := f.top.Mkdir(full, perm); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	if f.lowerHas(full) {
		n.opaque = true
		if err := f.nodes.store.SetOpaque(full); err != nil {
			return err
		}
	}
	if n.whiteout {
		n.whiteout = false
		if err := f.nodes.store.ClearWhiteout(full); err != nil {
			return err
		}
	}
	return nil
}

// Link creates a hard link newname → oldname. oldname is copied up if it is
// lower-only; the link is made within top.
func (f *Fs) Link(oldname, newname string) error {
	oldFull, err := f.resolve(oldname, false)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	if _, _, lerr := f.lookup(oldFull); lerr != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, lerr)
	}
	newFull, err := f.confine(newname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	if err := f.ensureCopiedUp(oldFull); err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}

	n := f.nodes.get(newFull)
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.whiteout {
		if _, err := f.top.Lstat(newFull); err == nil {
			return fsutil.WrapLinkErr("link", oldname, newname, fs.ErrExist)
		} else if !isNotExist(err) {
			return fsutil.WrapLinkErr("link", oldname, newname, err)
		}
		if !f.nodes.ancestorWhiteout(path.Dir(newFull)) &&
			!f.nodes.ancestorOpaque(newFull) && f.lowerHas(newFull) {
			return fsutil.WrapLinkErr("link", oldname, newname, fs.ErrExist)
		}
	}
	if err := f.ensureParentDirs(newFull); err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	if err := f.top.Link(oldFull, newFull); err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	if n.whiteout {
		n.whiteout = false
		if err := f.nodes.store.ClearWhiteout(newFull); err != nil {
			return fsutil.WrapLinkErr("link", oldname, newname, err)
		}
	}
	return nil
}

// Symlink creates a symlink newname with literal target oldname (oldname is not
// resolved).
func (f *Fs) Symlink(oldname, newname string) error {
	full, err := f.confine(newname)
	if err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	n := f.nodes.get(full)
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.whiteout {
		if _, err := f.top.Lstat(full); err == nil {
			return fsutil.WrapLinkErr("symlink", oldname, newname, fs.ErrExist)
		} else if !isNotExist(err) {
			return fsutil.WrapLinkErr("symlink", oldname, newname, err)
		}
		if !f.nodes.ancestorWhiteout(path.Dir(full)) &&
			!f.nodes.ancestorOpaque(full) && f.lowerHas(full) {
			return fsutil.WrapLinkErr("symlink", oldname, newname, fs.ErrExist)
		}
	}
	if err := f.ensureParentDirs(full); err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	if err := f.top.Symlink(oldname, full); err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	if n.whiteout {
		n.whiteout = false
		if err := f.nodes.store.ClearWhiteout(full); err != nil {
			return fsutil.WrapLinkErr("symlink", oldname, newname, err)
		}
	}
	return nil
}

// Remove deletes name. If name still resolves in a lower after the top removal,
// a whiteout is recorded (set BEFORE the top removal so a concurrent reader
// never sees the lower copy resurrected). Directory removal requires the merged
// directory to be empty.
func (f *Fs) Remove(name string) error {
	full, err := f.confine(name)
	if err != nil {
		return fsutil.WrapPathErr("remove", name, err)
	}
	n := f.nodes.get(full)
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.whiteout || f.nodes.ancestorWhiteout(path.Dir(full)) {
		return fsutil.WrapPathErr("remove", name, fs.ErrNotExist)
	}

	topInfo, topErr := f.top.Lstat(full)
	inTop := topErr == nil
	if topErr != nil && !isNotExist(topErr) {
		return fsutil.WrapPathErr("remove", name, topErr)
	}
	ancOpaque := f.nodes.ancestorOpaque(full)
	lowerVisible := !ancOpaque && f.lowerHas(full)
	if !inTop && !lowerVisible {
		return fsutil.WrapPathErr("remove", name, fs.ErrNotExist)
	}

	if f.opt.disableOpenFileRemoval && n.handles > 0 {
		return fsutil.WrapPathErr("remove", name, errSharingViolation)
	}

	info := topInfo
	if !inTop {
		_, info, _ = lowerLstat(f.lowers, full)
	}
	if info != nil && info.IsDir() {
		empty, err := f.mergedDirEmpty(full, !n.opaque && !ancOpaque)
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
		if !empty {
			return fsutil.WrapPathErr("remove", name, errdef.ENOTEMPTY)
		}
	}

	if lowerVisible {
		n.whiteout = true
		if err := f.nodes.store.SetWhiteout(full); err != nil {
			n.whiteout = false
			return fsutil.WrapPathErr("remove", name, err)
		}
	}
	if inTop {
		if err := f.top.Remove(full); err != nil && !isNotExist(err) {
			return fsutil.WrapPathErr("remove", name, err)
		}
	}
	if n.opaque {
		n.opaque = false
		_ = f.nodes.store.ClearOpaque(full)
	}
	return nil
}

// RemoveAll removes name and its subtree from top, then records a single
// subtree-masking whiteout if any lower content remains visible under name.
func (f *Fs) RemoveAll(name string) error {
	full, err := f.confine(name)
	if err != nil {
		return fsutil.WrapPathErr("removeall", name, err)
	}
	n := f.nodes.get(full)
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.whiteout || f.nodes.ancestorWhiteout(path.Dir(full)) {
		return nil
	}
	if f.opt.disableOpenFileRemoval && n.handles > 0 {
		return fsutil.WrapPathErr("removeall", name, errSharingViolation)
	}

	lowerVisible := !f.nodes.ancestorOpaque(full) && f.lowerHas(full)
	if lowerVisible {
		n.whiteout = true
		if err := f.nodes.store.SetWhiteout(full); err != nil {
			n.whiteout = false
			return fsutil.WrapPathErr("removeall", name, err)
		}
	}
	if err := f.top.RemoveAll(full); err != nil && !isNotExist(err) {
		return fsutil.WrapPathErr("removeall", name, err)
	}
	if n.opaque {
		n.opaque = false
		_ = f.nodes.store.ClearOpaque(full)
	}
	return nil
}

// Rename moves oldname to newname. A lower-only source is copied up first; the
// source name is whited out if it still resolves in a lower. The two involved
// nodes are locked in canonical lexical order to avoid deadlock with a
// concurrent reverse rename.
func (f *Fs) Rename(oldname, newname string) error {
	oldFull, err := f.confine(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	newFull, err := f.confine(newname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	if oldFull == newFull {
		if _, _, lerr := f.lookup(oldFull); lerr != nil {
			return fsutil.WrapLinkErr("rename", oldname, newname, lerr)
		}
		return nil
	}
	// Refuse moving a directory into its own subtree (matches os EINVAL).
	if strings.HasPrefix(newFull+"/", oldFull+"/") {
		return fsutil.WrapLinkErr("rename", oldname, newname, syscall.EINVAL)
	}

	if _, _, lerr := f.lookup(oldFull); lerr != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, lerr)
	}
	if err := f.ensureCopiedUp(oldFull); err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	// All masking decisions are taken BEFORE locking, so the locked section never
	// re-locks oldFull/newFull via an ancestor walk (D12).
	whiteoutOld := !f.nodes.ancestorOpaque(oldFull) && f.lowerHas(oldFull)

	a, b := oldFull, newFull
	if a > b {
		a, b = b, a
	}
	na := f.nodes.get(a)
	nb := f.nodes.get(b)
	na.mu.Lock()
	defer na.mu.Unlock()
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nOld := f.nodes.get(oldFull)
	nNew := f.nodes.get(newFull)

	if err := f.ensureParentDirs(newFull); err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	if err := f.top.Rename(oldFull, newFull); err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	if nNew.whiteout {
		nNew.whiteout = false
		_ = f.nodes.store.ClearWhiteout(newFull)
	}
	if whiteoutOld {
		nOld.whiteout = true
		_ = f.nodes.store.SetWhiteout(oldFull)
	}
	if nOld.opaque {
		nOld.opaque = false
		_ = f.nodes.store.ClearOpaque(oldFull)
	}
	return nil
}
