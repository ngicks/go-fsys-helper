package overlayfs

import (
	"io/fs"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// name normalizes an incoming path to the spelling the masking state is keyed
// by, joined onto the sub-root this view addresses.
//
// A path that is not local — absolute, or climbing past the root with ".." —
// escapes, whether it escapes the overlay or only the sub-root of an
// [Fs.OpenRoot] view.
func (f *Fs) name(p string) (string, error) {
	cleaned := filepath.Clean(p)
	if !filepath.IsLocal(cleaned) {
		return "", vroot.ErrPathEscapes
	}
	if cleaned == "." {
		return f.base, nil
	}
	return childName(f.base, filepath.ToSlash(cleaned)), nil
}

// resolver presents the merged view to [fsutil.ResolvePath], which walks a path
// component by component and follows every symlink it meets. Names are the
// caller's own — OS-form and relative to this view's root — so a link that
// climbs out of a sub-overlay is refused here rather than reaching the layers
// under it.
//
// Neither method locks: resolution runs inside an operation holding the state
// lock already.
type resolver struct {
	f *Fs
}

func (r resolver) Lstat(name string) (fs.FileInfo, error) {
	n, err := r.f.name(name)
	if err != nil {
		return nil, err
	}
	res, err := r.f.state.lookup(n)
	if err != nil {
		return nil, err
	}
	return res.info, nil
}

func (r resolver) ReadLink(name string) (string, error) {
	n, err := r.f.name(name)
	if err != nil {
		return "", err
	}
	res, err := r.f.state.lookup(n)
	if err != nil {
		return "", err
	}
	return res.src.fsys.ReadLink(res.path)
}

// resolve walks name over the merged view and returns where it lands, in the
// spelling the masking state is keyed by. skipLast leaves the final component
// alone, which is what an operation addressing a symlink itself wants.
//
// Resolution crosses layers: a link the top holds is followed into whatever
// layer its target resolves to, because every step of the walk asks the merged
// view rather than the layer the link came from.
func (f *Fs) resolve(name string, skipLast bool) (string, error) {
	resolved, err := fsutil.ResolvePath(resolver{f}, name, skipLast)
	if err != nil {
		return "", err
	}
	return f.name(resolved)
}

// locate is the step every read shares: resolve name over the merged view, then
// look up where it lands. It reports the resolved name as well — the spelling
// the masking state, and with it the open-handle count, is keyed by. The caller
// holds the state lock and wraps the error into the operation it is reporting.
func (f *Fs) locate(name string, skipLast bool) (string, lookupResult, error) {
	n, err := f.resolve(name, skipLast)
	if err != nil {
		return "", lookupResult{}, err
	}
	res, err := f.state.lookup(n)
	if err != nil {
		return "", lookupResult{}, err
	}
	return n, res, nil
}
