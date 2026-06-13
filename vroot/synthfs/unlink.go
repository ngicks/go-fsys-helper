package synthfs

// This file centralizes the two concerns that every unlink path (Remove,
// RemoveAll, Rename-over, and AddFile/AddFS override) shares:
//
//   - the DisableOpenFileRemoval refcount guard, and
//   - closing the unlinked FileView exactly once.
//
// Before this consolidation the guard lived only in Remove, so RemoveAll,
// Rename-over, and Add-override would close a view out from under still-open
// handles even when DisableOpenFileRemoval was set. Routing every unlink through
// these helpers makes the option's windows-like semantics uniform (plan 01 V3,
// decision D9).

// guardOpenRemoval reports a sharing-violation error when DisableOpenFileRemoval
// is set and n (or, for a directory, any descendant regular file) still has open
// handles. It is a pure pre-flight check: it mutates nothing, so callers can run
// it before any unlink to keep the operation atomic (RemoveAll in particular
// must reject the whole subtree before removing any of it).
//
// Caller holds state.mu.
func (r *Root) guardOpenRemoval(n node) error {
	if !r.st.opt.disableOpenFileRemoval {
		return nil
	}
	if hasOpenHandle(n) {
		return errSharingViolation
	}
	return nil
}

// hasOpenHandle reports whether n, or any regular file in its subtree, still has
// an open handle (refCount > 0). Caller holds state.mu.
func hasOpenHandle(n node) bool {
	switch nd := n.(type) {
	case *dir:
		for _, child := range nd.entriesSnapshot() {
			if hasOpenHandle(child) {
				return true
			}
		}
		return false
	default:
		return n.meta().refCount > 0
	}
}

// closeUnlinkedView closes the FileView attached to n when n is a regular file
// being unlinked from the tree. Symlinks and directories own no view, so it is a
// no-op for them. It does NOT consult the refcount guard — callers run
// guardOpenRemoval first when the operation honors DisableOpenFileRemoval.
//
// Caller holds state.mu.
func closeUnlinkedView(n node) {
	if f, ok := n.(*file); ok {
		_ = f.view.Close()
	}
}
