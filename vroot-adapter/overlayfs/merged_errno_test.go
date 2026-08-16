package overlayfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// The tests here run the merged view over layers that answer a failed path walk
// the way windows does, so the errno collision that platform has is exercised
// wherever the suite runs. On unix a backend says ENOENT for a name that is not
// there and ENOTDIR for one a non-directory stands in the way of; windows says
// ERROR_PATH_NOT_FOUND for both, whenever the component at fault is not the
// leaf, and that value is both what syscall.ENOTDIR spells there and one
// errors.Is reads as fs.ErrNotExist.

// winErrno is the pair of errno values a windows Lstat fails a path walk with,
// and how errors.Is reads them there.
type winErrno struct {
	// path distinguishes ERROR_PATH_NOT_FOUND, which is syscall.ENOTDIR on
	// windows, from ERROR_FILE_NOT_FOUND, which is syscall.ENOENT.
	path bool
}

func (e winErrno) Error() string {
	if e.path {
		return "The system cannot find the path specified."
	}
	return "The system cannot find the file specified."
}

func (e winErrno) Is(target error) bool {
	switch target {
	case fs.ErrNotExist:
		return true
	case syscall.ENOTDIR:
		return e.path
	}
	return false
}

// winFs answers Lstat as windows would: only a miss on the leaf itself is told
// apart from the rest.
type winFs struct {
	vroot.Fs[vroot.File]
}

func (f winFs) Lstat(name string) (fs.FileInfo, error) {
	info, err := f.Fs.Lstat(name)
	switch {
	case err == nil:
		return info, nil
	case errors.Is(err, syscall.ENOTDIR):
		return nil, winErrno{path: true}
	case errors.Is(err, fs.ErrNotExist):
		parent := filepath.Dir(name)
		if parent == name {
			return nil, winErrno{}
		}
		if _, err := f.Lstat(parent); err != nil {
			return nil, winErrno{path: true}
		}
		return nil, winErrno{}
	}
	return nil, err
}

// winLayers is [threeLayers] with every layer answering the windows way.
func winLayers(t *testing.T) (top, lower0, lower1 *DataSource) {
	t.Helper()
	top, lower0, lower1 = threeLayers(t)
	for _, d := range []*DataSource{top, lower0, lower1} {
		d.fsys = winFs{d.fsys}
	}
	return top, lower0, lower1
}

// TestOverlayStateShadowUnderWindowsErrno is
// [TestOverlayStateNonDirectoryShadow] over layers that cannot say in the errno
// what stopped the walk.
func TestOverlayStateShadowUnderWindowsErrno(t *testing.T) {
	top, lower0, lower1 := winLayers(t)
	seed(t, top, "blocked")
	seed(t, lower0, "blocked/child", "open/kept")
	seed(t, lower1, "open", "blocked/deeper")

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "blocked", 0)
	assertLookupErr(t, s, "blocked/child", syscall.ENOTDIR)
	assertLookupErr(t, s, "blocked/deeper", syscall.ENOTDIR)

	assertLayer(t, s, "open", 1)
	assertEntries(t, s, "open", "kept")
}

// TestOverlayStateMissingDirUnderWindowsErrno checks the other half of that one
// errno: a layer that simply has none of the directories on the way to a name
// has not blocked it, and the layers below go on answering for it.
func TestOverlayStateMissingDirUnderWindowsErrno(t *testing.T) {
	top, lower0, lower1 := winLayers(t)
	seed(t, top, "unrelated")
	seed(t, lower0, "deep/sub/file")
	seed(t, lower1, "deep/sub/other", "deep/sub/nested/leaf")

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "deep/sub/file", 1)
	assertLayer(t, s, "deep/sub/other", 2)
	assertLayer(t, s, "deep/sub/nested/leaf", 2)
	assertLookupErr(t, s, "deep/sub/missing", fs.ErrNotExist)
	assertEntries(t, s, "deep/sub", "file", "nested", "other")
}

// TestOverlayStateReadDirBlockedUnderWindowsErrno checks the merged listing on
// the same collision: a non-directory a lower holds at an ancestor of the
// listed name cuts that lower out, while a lower that merely lacks the
// ancestor goes on contributing.
func TestOverlayStateReadDirBlockedUnderWindowsErrno(t *testing.T) {
	top, lower0, lower1 := winLayers(t)
	seed(t, top, "a/b/from-top")
	seed(t, lower0, "a")
	seed(t, lower1, "a/b/from-lower1")

	s := newState(t, top, lower0, lower1)

	assertEntries(t, s, "a/b", "from-top")
	assertLookupErr(t, s, "a/b/from-lower1", fs.ErrNotExist)

	// The same stack with the blocking file gone: what lower1 holds was reachable
	// all along and only the non-directory in lower0 kept it out.
	top, lower0, lower1 = winLayers(t)
	seed(t, top, "a/b/from-top")
	seed(t, lower1, "a/b/from-lower1")

	s = newState(t, top, lower0, lower1)

	assertEntries(t, s, "a/b", "from-lower1", "from-top")
	assertLayer(t, s, "a/b/from-lower1", 2)
}
