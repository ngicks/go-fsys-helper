package overlayfs

import (
	"github.com/ngicks/go-fsys-helper/vroot"
)

// Layer bundles a backing filesystem with its overlay metadata journal.
//
// A Layer is consumed by [New] as either the writable top or one of the
// read-only lowers of a union mount. The backing fsys is stored type-erased as
// [vroot.Fs][vroot.File] (see [NewLayer]); the overlay calls only the plain
// [vroot.Fs] surface on it (reads on lowers, reads+writes on top).
//
// meta is the layer's metadata journal. In v1 the overlay uses ONLY the top
// layer's meta as its journal: it seeds the overlay virtual-node table from
// top.meta.Load() at construction and write-behinds whiteout/opaque changes to
// it. Lower layers' meta is accepted for API symmetry but is NOT consulted —
// lowers are immutable and read-only, so they carry no overlay deviation
// state. meta may be nil (typical for lowers, or for an ephemeral overlay whose
// top uses a no-op store); callers wiring a durable top supply a real store.
type Layer struct {
	// meta is the top layer's journal; nil-able. Not consulted for lowers.
	meta MetadataStore
	// fsys is the type-erased backing filesystem.
	fsys vroot.Fs[vroot.File]
}

// NewLayer bundles fsys with its metadata journal meta into a [Layer].
//
// The concrete file type F is erased to [vroot.File] via [vroot.Widen] (a
// no-op if fsys already satisfies [vroot.Fs][vroot.File], e.g. memfs or
// synthfs). Pass a rooted implementation (osfs.Root, memfs, synthfs.Root) so
// runtime confinement is preserved; the overlay enforces its own boundary
// regardless of whether the backing fsys is rooted.
//
// meta may be nil. See [Layer] for how meta is (and is not) used.
func NewLayer[F vroot.File](fsys vroot.Fs[F], meta MetadataStore) Layer {
	return Layer{
		meta: meta,
		fsys: vroot.Widen(fsys),
	}
}
