// Package sqlvfs implements the ncruces/go-sqlite3 VFS interfaces on top of a
// [vroot.Fs], so a sqlite database file lives inside an arbitrary vroot backend
// (osfs, memfs, sftpfs, ...) instead of at an OS path.
//
// Each [FS] is bound to one fsys instance and registers itself under a
// generated name; sqlite selects it through the "vfs=" URI parameter. Use
// [Open] to open (or create) a database inside an fsys with the locking and
// journal settings the overlay's metadata store needs.
package sqlvfs

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/util/vfsutil"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vfs.VFS = (*FS)(nil)

// FS is a [vfs.VFS] serving one [vroot.Fs].
//
// The sqlite VFS registry is keyed by name only, so an fsys-bound VFS cannot be
// registered once for the whole process the way memdb is; each FS takes a
// generated name instead and [FS.Unregister] gives it back.
type FS struct {
	name string
	fsys vroot.Fs[vroot.File]
}

// Register registers a VFS backed by fsys under a generated unique name and
// returns it. The caller keeps ownership of fsys: [FS.Unregister] does not
// close it.
func Register(fsys vroot.Fs[vroot.File]) *FS {
	f := &FS{
		name: "vroot-" + rand.Text(),
		fsys: fsys,
	}
	vfs.Register(f.name, f)
	return f
}

// Name returns the registered VFS name, as used in the "vfs=" URI parameter.
func (f *FS) Name() string {
	return f.name
}

// Unregister removes the VFS from the sqlite registry. Databases still open on
// it keep working; only new opens can no longer find it.
func (f *FS) Unregister() {
	vfs.Unregister(f.name)
}

// DSN returns the "file:" URI opening name, a path inside the fsys, through
// this VFS. Values of params are added to the query as is; repeated keys such
// as "_pragma" keep the order they are given in.
func (f *FS) DSN(name string, params url.Values) string {
	q := url.Values{"vfs": {f.name}}
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return (&url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     "/" + cleanPath(name),
		RawQuery: q.Encode(),
	}).String()
}

// Open implements [vfs.VFS].
func (f *FS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	if name == "" {
		// Unnamed files are private temporaries (sorter runs, statement journals).
		// OPEN_MEMORY tells sqlite they are neither locked nor persistent, so they
		// never have to reach the backing fsys.
		return &vfsutil.SliceFile{}, flags | vfs.OPEN_MEMORY, nil
	}

	oflag := os.O_RDONLY
	if flags&vfs.OPEN_READWRITE != 0 {
		oflag = os.O_RDWR
	}
	if flags&vfs.OPEN_CREATE != 0 {
		oflag |= os.O_CREATE
	}
	if flags&vfs.OPEN_EXCLUSIVE != 0 {
		oflag |= os.O_EXCL
	}

	name = cleanPath(name)
	file, err := f.fsys.OpenFile(name, oflag, 0o666)
	if err != nil {
		return nil, flags, vfs.SystemError(err, sqlite3.CANTOPEN)
	}
	return newFile(f.fsys, name, file, flags), flags, nil
}

// Delete implements [vfs.VFS].
func (f *FS) Delete(name string, syncDir bool) error {
	// syncDir is dropped: vroot has no directory handle to fsync, and the
	// backends that would need one (osfs) already order the unlink themselves.
	err := f.fsys.Remove(cleanPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return vfs.SystemError(err, sqlite3.IOERR_DELETE_NOENT)
	}
	return err
}

// Access implements [vfs.VFS].
func (f *FS) Access(name string, flags vfs.AccessFlag) (bool, error) {
	// ACCESS_READWRITE is answered as existence: vroot exposes no access(2)-style
	// permission probe, and sqlite only asks it for temp_store_directory.
	_, err := f.fsys.Stat(cleanPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FullPathname implements [vfs.VFS].
func (f *FS) FullPathname(name string) (string, error) {
	return cleanPath(name), nil
}

// cleanPath maps a sqlite pathname onto a path an [vroot.Fs] accepts: rooted
// implementations reject anything filepath.IsLocal rejects, and sqlite feeds
// FullPathname's answer back with "-journal" / "-wal" appended.
func cleanPath(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}
