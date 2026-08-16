package overlayfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// ErrNotCanonical reports a data source that does not carry the canonical
// layout and cannot be given it: a plain one handed to [New] as the top, or an
// fs whose "work" or "merged" is taken by something that is not a directory.
var ErrNotCanonical = errors.New("data source not canonical")

// The canonical layout, all of it directly inside the data source's fs root.
const (
	dirMerged = "merged"
	dirWork   = "work"
	storeName = "meta.sqlite3"
)

// DataSource is one layer of the overlay: a [vroot.Fs] together with the
// masking state that belongs to it.
//
// A plain data source, from [NewDataSource], is the fs as it stands — its root
// is the content root and it holds no masking state, which leaves it usable
// only as a lower. A canonical one, from [NewDataSourceCanonical] or
// [NewDataSourceCanonicalStore], keeps its content under "merged/", stages
// copy-ups under "work/" and owns a [MetadataStore]; it can serve as the top,
// or stack as a lower and go on masking the layers below it.
//
// A DataSource takes over what it is handed: [DataSource.Close] closes the
// store and the fs under it. Stack one layer under two overlays by sharing the
// *DataSource, not by building a second one over the same directory — that
// second one would block, not fail, for as long as the first holds the store's
// exclusive lock.
type DataSource struct {
	fsys      vroot.Fs[vroot.File]
	canonical bool
	store     MetadataStore
	// readOnly records that the store is open for reading only, which is what
	// keeps the layer out of the top slot in [New].
	readOnly bool
}

// NewDataSource wraps fsys as a plain data source: its root is the content
// root, with no "work/", no "merged/" and no masking state. Plain data sources
// are lower-only — [New] rejects one as the top with [ErrNotCanonical].
func NewDataSource[F vroot.File](fsys vroot.Fs[F]) *DataSource {
	return &DataSource{fsys: vroot.Widen(fsys)}
}

// NewDataSourceCanonical wraps fsys as a canonical data source, creating
// whatever of the layout is not there yet: the content root "merged/", the
// staging directory "work/", and the metadata store "meta.sqlite3" — the last
// through the vroot-backed sqlite VFS, so it lives inside fsys like the rest.
//
// The store is opened for writing, which is what a top needs and what holds the
// database exclusively until [DataSource.Close]. A backend that refuses that
// write — a read-only mount, a [vroot.ReadOnlyFs] — gets a read-only store
// instead and a data source [New] will only take as a lower. The mode is left
// to the backend to settle because nothing here knows yet which slot the layer
// will fill; a lower opened writable merely holds a lock it never uses.
func NewDataSourceCanonical[F vroot.File](fsys vroot.Fs[F]) (*DataSource, error) {
	wide := vroot.Widen(fsys)
	if err := ensureCanonical(wide); err != nil {
		return nil, err
	}
	store, readOnly, err := openStore(wide)
	if err != nil {
		return nil, err
	}
	return &DataSource{fsys: wide, canonical: true, store: store, readOnly: readOnly}, nil
}

// NewDataSourceCanonicalStore is [NewDataSourceCanonical] with store in place of
// the default sqlite one; nothing is opened or created at "meta.sqlite3". store
// is taken to be writable, the caller having picked it, and [DataSource.Close]
// closes it like the rest.
func NewDataSourceCanonicalStore[F vroot.File](
	fsys vroot.Fs[F],
	store MetadataStore,
) (*DataSource, error) {
	wide := vroot.Widen(fsys)
	if err := ensureCanonical(wide); err != nil {
		return nil, err
	}
	return &DataSource{fsys: wide, canonical: true, store: store}, nil
}

// Close closes the layer's metadata store and the fs under it.
func (d *DataSource) Close() error {
	var err error
	if d.store != nil {
		err = d.store.Close()
	}
	return errors.Join(err, d.fsys.Close())
}

// Fsys reports the [vroot.Fs] this layer wraps, which is the fs both
// [DataSource.ContentPath] and [DataSource.StagingPath] name a path in.
//
// The layer owns it: a [CopyUpPolicy] writes through it but must not close it,
// [DataSource.Close] being what closes it and the store beside it.
func (d *DataSource) Fsys() vroot.Fs[vroot.File] {
	return d.fsys
}

// ContentPath maps a merged-view name onto its path in this layer's content
// tree. name is the spelling the masking state is keyed by and a [CopyUpPolicy]
// is handed — clean, slash-separated, relative to the content root — and the
// answer is spelled with the separator the backend expects. A canonical layer
// keeps its content under "merged/"; a plain layer's root is its content root,
// so the name comes back as it went in.
//
// Content and staging are addressed as two paths in one fs rather than as two
// sub-Fs views, because copy-up publishes by renaming a staged file onto its
// content path and a rename cannot cross two Fs values.
func (d *DataSource) ContentPath(name string) string {
	name = filepath.FromSlash(name)
	if !d.canonical {
		return name
	}
	return filepath.Join(dirMerged, name)
}

// StagingPath maps name onto its path in this layer's staging tree "work/", in
// the terms [DataSource.ContentPath] takes and answers in.
//
// Only a canonical layer has a staging tree; a plain one answers "", a path
// addressing nothing that every fs method refuses. That answer never reaches a
// [CopyUpPolicy], which stages into the top and [New] admits no top but a
// canonical one, so it is left as a refusal rather than made an error the policy
// would have to carry.
func (d *DataSource) StagingPath(name string) string {
	if !d.canonical {
		return ""
	}
	return filepath.Join(dirWork, filepath.FromSlash(name))
}

// contentView addresses the layer's content tree as an fs of its own: what a
// [CopyUpPolicy] is handed as the layer to read from, where merged-view names
// are spelled as they stand instead of being joined onto a content root.
//
// The result borrows the layer's fs and must not be closed — a canonical
// layer's view is a [vroot.PathPrefixFs], whose Close closes the fs under it and
// with it the layer. The source is handed over as a view where the destination is
// handed over as the [DataSource] itself, because a policy only ever reads the
// source, by merged-view name, while the destination is where it has to tell
// content from staging — two trees in one fs, which is what publishing by rename
// needs.
func (d *DataSource) contentView() (vroot.Fs[vroot.File], error) {
	if !d.canonical {
		return d.fsys, nil
	}
	return vroot.NewPathPrefixFs(d.fsys, dirMerged)
}

// sweepWork empties the staging directory of whatever a previous run left in
// it. "work/" is overlay-private, so nothing there outlives the overlay that
// staged it; a layer with no staging directory has nothing to sweep rather than
// something to complain about.
func (d *DataSource) sweepWork() error {
	if !d.canonical {
		return nil
	}
	entries, err := vroot.ReadDir(d.fsys, dirWork)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := d.fsys.RemoveAll(filepath.Join(dirWork, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ensureCanonical creates the directories of the canonical layout that fsys
// does not have yet. Each is checked before it is created, not the other way
// around: a read-only backend rejects Mkdir even for a directory already there.
func ensureCanonical(fsys vroot.Fs[vroot.File]) error {
	for _, name := range []string{dirMerged, dirWork} {
		info, err := fsys.Stat(name)
		switch {
		case err == nil:
			if !info.IsDir() {
				return fmt.Errorf("overlayfs: %w: %q is not a directory", ErrNotCanonical, name)
			}
		case errors.Is(err, fs.ErrNotExist):
			if err := fsys.Mkdir(name, 0o777); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

// openStore opens the layer's own store, read-only if the backend will not have
// it any other way, and reports which of the two it got. Only a refused write
// earns the second attempt: a database this package cannot read fails read-only
// as well, and then the first error is the one that says why.
func openStore(fsys vroot.Fs[vroot.File]) (store MetadataStore, readOnly bool, err error) {
	writable, err := NewMetadataStoreSQLite(fsys, storeName)
	if err == nil {
		return writable, false, nil
	}
	if !errors.Is(err, errdef.EROFS) && !errors.Is(err, fs.ErrPermission) {
		return nil, false, err
	}
	frozen, roErr := newMetadataStoreSQLiteReadOnly(fsys, storeName)
	if roErr != nil {
		return nil, false, errors.Join(err, roErr)
	}
	return frozen, true, nil
}
