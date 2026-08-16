package overlayfs

import (
	"cmp"
	"errors"
	"io/fs"
	"slices"
	"syscall"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// lookupResult is the layer a merged-view name resolves to.
type lookupResult struct {
	// layer indexes the stack; 0 is the top.
	layer int
	// src is layers[layer] and path is where name sits in its content tree —
	// the pair every operation on the resolved file is addressed by.
	src  *DataSource
	path string
	// info is the Lstat the resolution already had to do.
	info fs.FileInfo
}

// lookup resolves name against the merged view: the shallowest layer that both
// shows name and is not masked for it.
//
// It reports [fs.ErrNotExist] when no layer shows name, whether it is missing
// everywhere or hidden by a mask, and [syscall.ENOTDIR] when the merged view
// holds a non-directory where name would need a directory. Callers wrap either
// into the [fs.PathError] their own operation reports. A caller that has to
// tell a whited-out name from an absent one asks [overlayState.marks] — the
// mask is state, not part of the answer.
func (s *overlayState) lookup(name string) (lookupResult, error) {
	return s.lookupFrom(name, 0)
}

// lookupFrom is [overlayState.lookup] restricted to layers at or below from. It
// answers what the merged view would show if the layers above gave name up,
// which is the question a removal asks before it decides whether a whiteout is
// needed at all.
//
// Gave it up entirely, masking included: the bound comes from
// [overlayState.cutFrom] rather than [overlayState.cut], so the marks of the
// layers above — the very marks the caller is deciding — do not answer the
// question put to the layers below.
func (s *overlayState) lookupFrom(name string, from int) (lookupResult, error) {
	name = cleanName(name)
	cut := s.cutFrom(name, from)
	for i := from; i <= cut; i++ {
		path := s.layerPath(i, name)
		info, err := s.layers[i].fsys.Lstat(path)
		switch {
		case err == nil:
			return lookupResult{layer: i, src: s.layers[i], path: path, info: info}, nil
		case errors.Is(err, fs.ErrNotExist):
			continue
		case errors.Is(err, syscall.ENOTDIR):
			// Not this layer's business alone: a non-directory on the way to name
			// fills a path every deeper layer would have to be reached through, so
			// the search ends here instead of stepping around it.
			return lookupResult{}, s.blockedErr(name)
		default:
			// Anything else is the backend failing rather than answering, and a
			// layer that cannot answer must not be read as an empty one.
			return lookupResult{}, err
		}
	}
	return lookupResult{}, fs.ErrNotExist
}

// blockedErr names what a non-directory component left behind: the merged view
// really does hold a non-directory where name's parent should be, or the parent
// is a directory contributed by a layer above the one that blocked the search
// and name is merely absent from it.
func (s *overlayState) blockedErr(name string) error {
	if name == "" {
		return syscall.ENOTDIR
	}
	parent, err := s.lookup(parentName(name))
	if err == nil && parent.info.IsDir() {
		return fs.ErrNotExist
	}
	return syscall.ENOTDIR
}

// readDir merges what the layers hold at the directory name, sorted by name.
// The shallowest layer showing an entry wins it whole — its type and its
// backing layer come from there — and the layers below stop contributing at the
// first of: a whiteout on that entry, an opaque mark on name, a non-directory
// standing in name's place.
//
// Entries are the layers' own, so an [fs.DirEntry.Info] call reaches the layer
// that produced it rather than the merged view.
func (s *overlayState) readDir(name string) ([]fs.DirEntry, error) {
	name = cleanName(name)
	res, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	if !res.info.IsDir() {
		return nil, syscall.ENOTDIR
	}

	var entries []fs.DirEntry
	seen := make(map[string]struct{})
	cut := s.childCut(name)
	for i := res.layer; i <= cut; i++ {
		path := s.layerPath(i, name)
		info, err := s.layers[i].fsys.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case errors.Is(err, syscall.ENOTDIR), err == nil && !info.IsDir():
			// A non-directory at or on the way to name fills the path the layers
			// below would be merged in through: nothing under them is part of this
			// directory any more.
			return sortEntries(entries), nil
		case err != nil:
			return nil, err
		}
		read, err := vroot.ReadDir(s.layers[i].fsys, path)
		if err != nil {
			return nil, err
		}
		for _, e := range read {
			if _, ok := seen[e.Name()]; ok {
				continue
			}
			if s.whiteoutAbove(i, childName(name, e.Name())) {
				continue
			}
			seen[e.Name()] = struct{}{}
			entries = append(entries, e)
		}
	}
	return sortEntries(entries), nil
}

func sortEntries(entries []fs.DirEntry) []fs.DirEntry {
	slices.SortFunc(
		entries,
		func(i, j fs.DirEntry) int { return cmp.Compare(i.Name(), j.Name()) },
	)
	return entries
}
