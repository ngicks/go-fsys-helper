// Package overlayfs implements a union mount over vroot layers: a writable top
// stacked on one or more read-only lowers, exposed as a vroot.Root.
//
// Reads resolve in mount order (top, then lowers[0], lowers[1], ...); writes
// land in the top layer; a lower-only file is copied up before it is modified.
// Deletion state (whiteout/opaque) is kept in a sqlite metadata store beside the
// layer's data, never as marker files inside the layer itself.
package overlayfs
