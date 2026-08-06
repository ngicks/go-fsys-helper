package overlayfs

import (
	"errors"
	"io/fs"
	"path"
	"syscall"
)

// ensureCopiedUp guarantees that full exists in top, copying it up from its
// lower owner if it currently lives only in a lower. It is the precondition for
// any mutating op that needs to act on a top-resident copy (write-open of an
// existing file, Chmod/Chown/Chtimes/Lchown, Rename/Link of a lower-only
// source).
//
// If full already exists in top it is a no-op. If full exists in no layer it is
// also a no-op (the caller is creating a fresh entry). Otherwise the copy runs
// inside nodes.copyUp (singleflight, decision 11) so concurrent writers to the
// same lower-only path collapse to a single copy; NO node lock is held across
// the copy I/O.
func (f *Fs) ensureCopiedUp(full string) error {
	if _, err := f.top.Lstat(full); err == nil {
		return nil
	} else if !isNotExist(err) {
		return err
	}
	winLower, _, err := lowerLstat(f.lowers, full)
	if err != nil {
		if isNotExist(err) {
			return nil // nothing to copy up; caller creates fresh
		}
		return err
	}
	return f.nodes.copyUp(full, func() error {
		// Re-check under the singleflight: a concurrent copyUp may have already
		// materialized full in top.
		if _, err := f.top.Lstat(full); err == nil {
			return nil
		}
		if err := f.ensureParentDirs(full); err != nil {
			return err
		}
		return f.policy.CopyUp(winLower, f.top, full)
	})
}

// ensureParentDirs makes every ancestor directory of full exist in top,
// top-down and NON-opaque (lower siblings stay visible — PLAN §5.3 step 2). An
// ancestor already in top is kept; one that lives only in a lower is copied up
// (created in top with the lower dir's perm). An ancestor that exists in NO
// layer — or is masked by a whiteout, or is a non-directory — makes this fail
// with fs.ErrNotExist / ENOTDIR, exactly as os.Mkdir/os.OpenFile do when a path
// component is missing. The final component (full itself) is never created.
//
// This is the single gate for "parent must exist": create ops (Mkdir, Create,
// OpenFile O_CREATE, Symlink, Link, Rename target) call it and surface its
// error, while copy-up callers always pass a full whose ancestors do exist in a
// lower, so it materializes them.
func (f *Fs) ensureParentDirs(full string) error {
	parent := path.Dir(full)
	if parent == "." || parent == "/" || parent == full {
		return nil
	}
	// A whited-out ancestor means the parent chain was deleted → ENOENT.
	if f.nodes.ancestorWhiteout(parent) {
		return fs.ErrNotExist
	}
	for _, anc := range pathChain(parent) {
		if info, err := f.top.Lstat(anc); err == nil {
			if !info.IsDir() {
				return syscall.ENOTDIR
			}
			continue // already present in top
		} else if !isNotExist(err) {
			return err
		}
		_, info, lerr := lowerLstat(f.lowers, anc)
		if lerr != nil {
			if isNotExist(lerr) {
				return fs.ErrNotExist // ancestor exists in no layer
			}
			return lerr
		}
		if !info.IsDir() {
			return syscall.ENOTDIR
		}
		if err := f.top.Mkdir(anc, info.Mode().Perm()); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}
