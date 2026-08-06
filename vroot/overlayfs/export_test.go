package overlayfs

// This file exposes unexported overlay internals to the external _test package
// (overlayfs_test) so read-path tests can assert on the ovl-node table without
// widening the public API. It is compiled only under `go test`.

// HandleCountForTest returns the live open-handle count the overlay's ovl-node
// table records for the full top-relative path p.
func HandleCountForTest(f *Fs, p string) int {
	return f.nodes.handleCount(p)
}

// HasNodeForTest reports whether the overlay's ovl-node table currently has an
// interned node for the full top-relative path p (i.e. the node was not GC'd).
func HasNodeForTest(f *Fs, p string) bool {
	_, ok := f.nodes.peek(p)
	return ok
}
