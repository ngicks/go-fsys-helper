package overlayfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// Mkdir creates the directory name in the top.
//
// It fails with [fs.ErrExist] when any layer already shows name, and marks the
// new directory opaque unconditionally, following the reference's
// ovl_mkdir(set_opaque=true) (D7). That is what makes "remove a directory, make
// it again" leave the lower children of the old one hidden, without a rule of
// its own for recreating over a whiteout.
func (f *Fs) Mkdir(name string, perm fs.FileMode) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapPathErr("mkdir", name, f.mkdir(name, perm))
}

func (f *Fs) mkdir(name string, perm fs.FileMode) error {
	full, err := f.resolve(name, true)
	if err != nil {
		return err
	}
	switch _, err := f.state.lookup(full); {
	case err == nil:
		return fs.ErrExist
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := f.ensureParent(full); err != nil {
		return err
	}
	return f.mkdirOne(full, perm)
}

// mkdirOne creates one directory in the top and gives it the masking a fresh
// directory carries.
//
// The opaque mark goes on before the whiteout that stood on the name comes off,
// so a crash between the two leaves a masked name rather than a directory with
// the leftovers of whatever used to be there merged into it.
func (f *Fs) mkdirOne(full string, perm fs.FileMode) error {
	if err := f.top().fsys.Mkdir(f.topPath(full), perm); err != nil {
		return err
	}
	if err := f.state.setOpaque(full); err != nil {
		return err
	}
	return f.unmask(full)
}

// MkdirAll creates name and every directory above it that is missing.
//
// A directory some layer already shows is left as it is — no error, and no
// change to the masking around it. Every directory this call brings into being
// is a fresh one and is marked opaque like [Fs.Mkdir]'s (D7); that is the
// difference from the directories a copy-up materializes in passing, which are
// never opaque because they were not created for their own sake.
func (f *Fs) MkdirAll(name string, perm fs.FileMode) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapPathErr("mkdir", name, f.mkdirAll(name, perm))
}

func (f *Fs) mkdirAll(name string, perm fs.FileMode) error {
	full, err := f.mkdirAllName(name)
	if err != nil {
		return err
	}
	for _, comp := range ancestry(full) {
		switch res, err := f.state.lookup(comp); {
		case err == nil && res.info.IsDir():
			continue
		case err == nil:
			return syscall.ENOTDIR
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		if err := f.ensureParent(comp); err != nil {
			return err
		}
		if err := f.mkdirOne(comp, perm); err != nil {
			return err
		}
	}
	return nil
}

// mkdirAllName is [Fs.resolve] for a path most of which is not there yet.
//
// [fsutil.ResolvePath] gives up at the first component it cannot Lstat, which for
// MkdirAll is the normal case rather than a failure, so the walk is retried on
// shorter and shorter prefixes until one lands and the components trimmed off are
// put back. The symlinks in the part that exists are followed; what does not
// exist has none to follow.
func (f *Fs) mkdirAllName(name string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if cleaned == "." {
		return f.base, nil
	}
	if !filepath.IsLocal(filepath.FromSlash(cleaned)) {
		return "", vroot.ErrPathEscapes
	}
	for head, rest := cleaned, ""; ; {
		resolved, err := f.resolve(filepath.FromSlash(head), true)
		if err == nil {
			return childName(resolved, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		i := strings.LastIndexByte(head, '/')
		if i < 0 {
			return "", err
		}
		rest = childName(head[i+1:], rest)
		head = head[:i]
	}
}
