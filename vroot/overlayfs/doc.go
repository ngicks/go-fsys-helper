// Package overlayfs implements a union mount: a writable top layer stacked
// over one or more read-only lower layers, exposed as a
// [vroot.Root][vroot.File, *Fs].
//
// Reads resolve top-down (top wins); writes land in top; a lower-only file is
// copied up before it is modified; deletions are recorded as whiteouts (a
// path and its lower subtree are hidden) or opaques (a top dir whose lower
// children are hidden) instead of mutating the immutable lowers.
//
// Overlay deviation-state (whiteout/opaque/copy-up/open-handle bookkeeping)
// lives in a sparse, lazily built table of overlay virtual nodes that doubles
// as the cross-layer transaction boundary. Durable whiteout/opaque records are
// kept side-band in a pluggable [MetadataStore]: [MetadataStoreMem] for
// ephemeral overlays and [MetadataStoreLog] (an append-only, self-compacting
// journal) for the durable case.
package overlayfs
