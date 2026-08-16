package overlayfs

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

// Rename moves oldname to newname.
//
// A file the top does not hold is copied up first and the move happens inside the
// top; the old name is whited out when the layers below still show it. A
// directory the layers below contribute to is refused with EXDEV — see
// [Fs.renameDirCheck].
//
// Renaming onto a name that is a non-empty directory in the merged view fails
// with ENOTEMPTY even when the entries filling it come from a lower.
func (f *Fs) Rename(oldname string, newname string) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapLinkErr("rename", oldname, newname, f.rename(oldname, newname))
}

func (f *Fs) rename(oldname string, newname string) error {
	if namesSelf(oldname) || namesSelf(newname) {
		return syscall.EINVAL
	}
	oldFull, err := f.resolve(oldname, true)
	if err != nil {
		return err
	}
	newFull, err := f.resolve(newname, true)
	if err != nil {
		return err
	}
	if oldFull == "" || newFull == "" {
		return syscall.EINVAL
	}
	oldRes, err := f.state.lookup(oldFull)
	if err != nil {
		return err
	}
	if oldFull == newFull {
		return nil
	}
	// Moving a directory into itself would cut the tree loose from the root;
	// rename(2) answers EINVAL and so does this.
	if strings.HasPrefix(newFull+"/", oldFull+"/") {
		return syscall.EINVAL
	}
	if oldRes.info.IsDir() {
		if err := f.renameDirCheck(oldFull); err != nil {
			return err
		}
	}
	if err := f.renameTargetCheck(newFull, oldRes.info.IsDir()); err != nil {
		return err
	}

	if err := f.copyUp(oldFull); err != nil {
		return err
	}
	if err := f.ensureParent(newFull); err != nil {
		return err
	}
	// Asked before the move: afterwards the layers below cannot be asked what they
	// still show at a name the top has given up.
	maskOld := false
	switch _, err := f.state.lookupFrom(oldFull, 1); {
	case err == nil:
		maskOld = true
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	if err := f.top().fsys.Rename(f.topPath(oldFull), f.topPath(newFull)); err != nil {
		return err
	}

	// Whatever stood at the new name is gone, and the marks describing it go with
	// it — the whiteout a removal there left included, which is what lets a name
	// be renamed onto one that was deleted.
	if err := f.state.clearSubtree(newFull); err != nil {
		return err
	}
	if err := f.state.renameSubtree(oldFull, newFull); err != nil {
		return err
	}
	if oldRes.info.IsDir() {
		// The reference's create_missing_whiteouts, in the store's own terms: it
		// walks the layers below at the destination and drops a whiteout file
		// inside the moved directory for every entry they show that the moved
		// directory does not, recursing into the subdirectories it does
		// (C main.c:2456-2530 @ 4759abd, called from main.c:4830). One opaque mark
		// says the same thing about the whole subtree in one row, and says it
		// about the names the moved directory does provide as well — which the
		// per-entry form leaves uncovered, so that removing one of them there
		// brings a lower entry back.
		if err := f.state.setOpaque(newFull); err != nil {
			return err
		}
	}
	if maskOld {
		// setWhiteout drops the old name's own opaque mark and everything recorded
		// under it, which renameSubtree has already moved out of the way. A
		// whiteout at the new name is not needed however the layers below stand:
		// the top now holds content there, and content in the top shadows them.
		return f.state.setWhiteout(oldFull)
	}
	return f.state.clearSubtree(oldFull)
}

// renameDirCheck refuses a directory the layers below still contribute to.
//
// The reference answers EXDEV for exactly this (Rust overlay.rs:2251-2265; C
// main.c:4756-4763 @ 4759abd), because moving such a directory means either
// copying its whole lower subtree up or leaving a redirect behind for the old
// path, and it does neither. Nor does this: a rename that quietly deep-copied a
// lower tree would turn a metadata operation into an unbounded one, and a
// redirect is a second masking concept the store has no room for. A directory the
// top holds alone moves as the one rename it looks like.
func (f *Fs) renameDirCheck(full string) error {
	cut := f.state.childCut(full)
	for i := 1; i <= cut; i++ {
		switch _, err := f.state.layers[i].fsys.Lstat(f.state.layerPath(i, full)); {
		case err == nil:
			return errCrossDevice
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
			continue
		default:
			return err
		}
	}
	return nil
}

// renameTargetCheck reports what the merged view says about renaming onto full,
// which the top alone cannot answer: the directory standing there may be filled
// entirely from a lower, and the top's own rename would happily replace the
// nothing it sees.
func (f *Fs) renameTargetCheck(full string, srcIsDir bool) error {
	res, err := f.state.lookup(full)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case res.info.IsDir() && !srcIsDir:
		return syscall.EISDIR
	case !res.info.IsDir() && srcIsDir:
		return syscall.ENOTDIR
	case !res.info.IsDir():
		return nil
	}
	entries, err := f.state.readDir(full)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return errdef.ENOTEMPTY
	}
	return nil
}
