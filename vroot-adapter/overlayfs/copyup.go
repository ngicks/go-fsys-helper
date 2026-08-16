package overlayfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// ErrTypeNotSupported reports a file whose type a copy-up cannot reproduce:
// a device, a socket, a named pipe. [vroot.Fs] has no method that creates one,
// so such a file can be read where it lies but never brought into the top.
var ErrTypeNotSupported = errors.New("type not supported")

// CopyUpPolicy materializes a lower-backed name in the top layer, which is what
// every write to a name the top does not hold has to do first.
//
// # Contract
//
// name is a merged-view path in the form the masking state is keyed by: clean,
// slash-separated, relative to the content root, and never "" — the root is
// always already there. from is a content-rooted view of the layer name
// currently resolves to, so from and name address the source file together: a
// plain layer's own fs, and a canonical layer's "merged/" wrapped in a
// [vroot.PathPrefixFs]. top is canonical, and the policy writes into its content
// tree and stages in whatever place it picks: [DataSource.Fsys] is the fs holding
// both trees, [DataSource.ContentPath] is where name belongs in the content one
// and [DataSource.StagingPath] where it would go under "work/" — one fs, which is
// what lets a staged file be published onto its content path by rename. Those
// three are all the default policy uses, so one written outside this package has
// the same reach.
//
// The policy owns nothing it is handed: neither from nor top may be closed —
// from may be a view whose Close would close the layer under it — and neither
// is kept past the call.
//
// The caller has already resolved name and found the top without it, and holds
// the overlay's write lock for the whole call: the policy runs alone and does
// not lock. Masking is the caller's as well — a policy records none of it, and
// the parent directories it has to create along the way are ordinary
// directories, never opaque (D7).
type CopyUpPolicy interface {
	CopyUp(from vroot.Fs[vroot.File], top *DataSource, name string) error
}

var _ CopyUpPolicy = (*CopyUpPolicyWork)(nil)

// CopyUpPolicyWork is the default policy: a regular file is staged as a temp in
// the top's "work/", filled by reflink where the backends allow one and by a
// buffered copy otherwise, and published by renaming it onto its content path —
// atomic, because staging and content are two paths in one fs. Directories and
// symlinks are created in the content tree directly, having no data to stage.
//
// The zero value is ready to use; [NewCopyUpPolicyWork] is the spelling to
// prefer. One policy is safe to share between overlays.
type CopyUpPolicyWork struct {
	// temp numbers the staged files. Names are counted rather than randomized
	// because "work/" is overlay-private and swept at [New]: the only writer
	// that could collide is another overlay over the same top, which the store's
	// exclusive lock already rules out.
	temp atomic.Uint64
}

// NewCopyUpPolicyWork returns the default copy-up policy.
func NewCopyUpPolicyWork() *CopyUpPolicyWork {
	return &CopyUpPolicyWork{}
}

// CopyUp implements [CopyUpPolicy].
func (p *CopyUpPolicyWork) CopyUp(from vroot.Fs[vroot.File], top *DataSource, name string) error {
	name = cleanName(name)
	if name == "" {
		return fmt.Errorf("overlayfs: copy up %q: %w", name, fs.ErrInvalid)
	}
	info, err := from.Lstat(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	// Refused before anything is created, so a type this cannot carry over
	// leaves the top exactly as it found it rather than holding the parents of a
	// file that never arrives.
	if typ := info.Mode().Type(); typ&^(fs.ModeDir|fs.ModeSymlink) != 0 {
		return fmt.Errorf("overlayfs: copy up %q: %w: %s", name, ErrTypeNotSupported, typ)
	}

	if err := p.materializeParents(from, top, parentName(name)); err != nil {
		return err
	}
	dst := top.ContentPath(name)
	switch {
	case info.IsDir():
		// Only the directory itself: its children stay where they are and go on
		// being merged in from below, which is what keeps a copy-up from walking
		// a whole subtree to write one file in it.
		if err := top.fsys.Mkdir(dst, info.Mode().Perm()); err != nil {
			return err
		}
		copyAttrs(top.fsys, dst, info)
		return nil
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := from.ReadLink(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		// No mode or times: vroot has no Lchmod/Lchtimes, and the Chmod and
		// Chtimes it does have follow the link — they would rewrite whatever the
		// new symlink points at instead of the link.
		return top.fsys.Symlink(target, dst)
	default:
		return p.copyRegular(from, top, name, info)
	}
}

// copyRegular stages name's data under "work/" and publishes it by rename.
func (p *CopyUpPolicyWork) copyRegular(
	from vroot.Fs[vroot.File],
	top *DataSource,
	name string,
	info fs.FileInfo,
) error {
	src, err := from.Open(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	staging := top.StagingPath("tmp-" + strconv.FormatUint(p.temp.Add(1), 10))
	tmp, err := top.fsys.OpenFile(staging, os.O_RDWR|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			// Nothing addresses a staged file that never got published, so it
			// would sit in "work/" until the next New swept it.
			_ = top.fsys.Remove(staging)
		}
	}()

	if !reflinkFile(tmp, src) {
		if err := copyData(tmp, src); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	copyAttrs(top.fsys, staging, info)
	if err := top.fsys.Rename(staging, top.ContentPath(name)); err != nil {
		return err
	}
	published = true
	return nil
}

// materializeParents creates the directories name needs above it that the top
// does not hold yet, shallowest first, taking each one's mode from the layer the
// copy-up reads.
//
// They are plain directories: an opaque mark would hide the lower children of a
// directory this only had to create in passing (D7).
func (p *CopyUpPolicyWork) materializeParents(
	from vroot.Fs[vroot.File],
	top *DataSource,
	dir string,
) error {
	if dir == "" {
		return nil
	}
	if err := p.materializeParents(from, top, parentName(dir)); err != nil {
		return err
	}
	path := top.ContentPath(dir)
	switch _, err := top.fsys.Lstat(path); {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	// A parent the source layer does not describe still has to exist for the
	// copy-up to land, so it is created open and left for a later chmod rather
	// than failing an operation that is only passing through here.
	perm := fs.FileMode(0o777)
	info, err := from.Stat(filepath.FromSlash(dir))
	described := err == nil
	if described {
		perm = info.Mode().Perm()
	}
	if err := top.fsys.Mkdir(path, perm); err != nil {
		return err
	}
	if described {
		copyAttrs(top.fsys, path, info)
	}
	return nil
}

// copyAttrs brings path's mode and times in line with info.
//
// Failures are dropped: the data is already copied, and a backend that keeps no
// timestamps or refuses a chmod would otherwise fail a copy-up that did
// everything it was asked for. Mode is re-applied even where the create already
// carried it, since a umask may have taken bits off it. atime is set to
// info.ModTime() for want of anything better — [fs.FileInfo] reports no access
// time portably.
func copyAttrs(fsys vroot.Fs[vroot.File], path string, info fs.FileInfo) {
	_ = fsys.Chmod(path, info.Mode().Perm())
	_ = fsys.Chtimes(path, info.ModTime(), info.ModTime())
}

const copyBufSize = 64 * 1024

// copyBufPool mirrors fsutil's bufpool, which is internal to the fsutil module
// and so cannot be imported from here.
var copyBufPool = &sync.Pool{
	New: func() any {
		b := make([]byte, copyBufSize)
		return &b
	},
}

// copyData is the fallback for everything reflink did not take: the plain
// buffered loop of D9, with no sendfile or copy_file_range tier of this
// package's own. io.CopyBuffer may still reach one through a backend's own
// ReadFrom or WriteTo, which is the backend's business rather than a tier here.
func copyData(dst io.Writer, src io.Reader) error {
	buf := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(buf)
	_, err := io.CopyBuffer(dst, src, *buf)
	return err
}
