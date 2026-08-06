package overlayfs

import (
	"path"
	"sync"

	"golang.org/x/sync/singleflight"
)

// upState is the copy-up state of an [ovlNode]. It is re-derivable from
// top.Lstat (a copied-up path exists in top), so it is a self-healing hint
// rather than trusted state, and nodes carrying only this state are GC-able
// (decision 16).
type upState int

const (
	// upUnknown is the zero value: copy-up state has not been determined.
	upUnknown upState = iota
	// upNotCopied means the path is known to live only in a lower (not in top).
	upNotCopied
	// upCopied means the path has been copied up to top.
	upCopied
)

// ovlNode is the overlay virtual node for a single touched path. It owns the
// overlay's per-path deviation-state (whiteout/opaque/copy-up) and open-handle
// accounting, and its mutex is the cross-layer transaction boundary
// (PLAN §5.0). Nodes are interned lazily in an [ovlNodes] table.
type ovlNode struct {
	// mu guards every field of this node. It is the per-path lock; the table's
	// lock (ovlNodes.mu) guards only the map structure and is never held across
	// I/O nor while a node lock is held for I/O.
	mu sync.RWMutex
	// path is the clean slash, full top-relative key this node represents.
	path string

	// whiteout, when set, hides path and the whole lower subtree beneath it.
	// Authoritative in-memory; mirrored to the journal.
	whiteout bool
	// opaque, when set, means path is a dir present in top whose lower children
	// are hidden (top children remain visible). Authoritative; mirrored to the
	// journal.
	opaque bool

	// up is the copy-up hint (re-derivable from top.Lstat).
	up upState

	// handles counts live open handles referencing this path. It backs the
	// Windows ERROR_SHARING_VIOLATION emulation and keeps a node from being
	// GC'd while open.
	handles int
}

// ovlNodes is the interning table of overlay virtual nodes. It is the
// authoritative in-memory source of overlay deviation-state and the write-behind
// owner of the metadata journal.
type ovlNodes struct {
	// mu guards byPath (the map structure) ONLY. It is never held across I/O.
	mu     sync.RWMutex
	byPath map[string]*ovlNode
	// cow dedups concurrent copy-ups keyed by path (decision 11). fn runs once
	// for concurrent callers; no node lock is held across it.
	cow singleflight.Group
	// store is the write-behind journal for whiteout/opaque (decision 8). It is
	// the durable mirror of the authoritative node bits.
	store MetadataStore
}

// newOvlNodes builds an interning table seeded from store. store must be
// non-nil; the caller substitutes [MetadataStoreMem] when none is configured.
//
// store.Load() is consulted once: a node is interned per whiteout path (with
// whiteout=true) and per opaque path (with opaque=true). A Load error is
// returned; the caller decides how to surface it (see [New], which starts with
// an empty seeded table on Load failure).
func newOvlNodes(store MetadataStore) (*ovlNodes, error) {
	nt := &ovlNodes{
		byPath: make(map[string]*ovlNode),
		store:  store,
	}
	whiteouts, opaques, err := store.Load()
	if err != nil {
		return nt, err
	}
	for _, p := range whiteouts {
		n := nt.get(p)
		n.mu.Lock()
		n.whiteout = true
		n.mu.Unlock()
	}
	for _, p := range opaques {
		n := nt.get(p)
		n.mu.Lock()
		n.opaque = true
		n.mu.Unlock()
	}
	return nt, nil
}

// get returns the node for p, interning (get-or-create) it if absent. The table
// lock is taken only for the lookup/insert (double-checked); no I/O happens
// under it.
func (nt *ovlNodes) get(p string) *ovlNode {
	nt.mu.RLock()
	n, ok := nt.byPath[p]
	nt.mu.RUnlock()
	if ok {
		return n
	}
	nt.mu.Lock()
	defer nt.mu.Unlock()
	// Double-check: another goroutine may have interned p between the RUnlock
	// and the Lock.
	if n, ok = nt.byPath[p]; ok {
		return n
	}
	n = &ovlNode{path: p}
	nt.byPath[p] = n
	return n
}

// peek returns the node for p only if it is already interned; it never creates.
func (nt *ovlNodes) peek(p string) (*ovlNode, bool) {
	nt.mu.RLock()
	n, ok := nt.byPath[p]
	nt.mu.RUnlock()
	return n, ok
}

// ancestorWhiteout reports whether p or any strict ancestor of p carries a
// whiteout bit (whiteout masks the whole lower subtree beneath it). Components
// are walked "a", "a/b", … and each interned node is peeked + read under its
// RLock.
func (nt *ovlNodes) ancestorWhiteout(p string) bool {
	for _, anc := range pathChain(p) {
		if n, ok := nt.peek(anc); ok {
			n.mu.RLock()
			w := n.whiteout
			n.mu.RUnlock()
			if w {
				return true
			}
		}
	}
	return false
}

// ancestorOpaque reports whether any STRICT ancestor dir of p carries an opaque
// bit (opaque hides the whole lower subtree beneath it for resolution). p's own
// opaque bit is NOT consulted here — opaque at p hides p's lower children, not
// p itself.
func (nt *ovlNodes) ancestorOpaque(p string) bool {
	for _, anc := range pathChain(p) {
		if anc == p {
			continue
		}
		if n, ok := nt.peek(anc); ok {
			n.mu.RLock()
			o := n.opaque
			n.mu.RUnlock()
			if o {
				return true
			}
		}
	}
	return false
}

// opaqueAt reports whether the dir at dir carries an opaque bit (its lower
// children are hidden).
func (nt *ovlNodes) opaqueAt(dir string) bool {
	n, ok := nt.peek(dir)
	if !ok {
		return false
	}
	n.mu.RLock()
	o := n.opaque
	n.mu.RUnlock()
	return o
}

// setWhiteout sets p's whiteout bit then write-behinds it to the journal. The
// caller is responsible for the higher-level invariants (e.g. holding the
// path's lock across the cross-layer transaction).
func (nt *ovlNodes) setWhiteout(p string) error {
	n := nt.get(p)
	n.mu.Lock()
	n.whiteout = true
	n.mu.Unlock()
	return nt.store.SetWhiteout(p)
}

// clearWhiteout clears p's whiteout bit then write-behinds it to the journal.
func (nt *ovlNodes) clearWhiteout(p string) error {
	n := nt.get(p)
	n.mu.Lock()
	n.whiteout = false
	n.mu.Unlock()
	return nt.store.ClearWhiteout(p)
}

// setOpaque sets p's opaque bit then write-behinds it to the journal.
func (nt *ovlNodes) setOpaque(p string) error {
	n := nt.get(p)
	n.mu.Lock()
	n.opaque = true
	n.mu.Unlock()
	return nt.store.SetOpaque(p)
}

// clearOpaque clears p's opaque bit then write-behinds it to the journal.
func (nt *ovlNodes) clearOpaque(p string) error {
	n := nt.get(p)
	n.mu.Lock()
	n.opaque = false
	n.mu.Unlock()
	return nt.store.ClearOpaque(p)
}

// copyUp runs fn at most once for concurrent callers on the same p
// (singleflight, decision 11). No node lock is held across fn (fn performs the
// copy I/O). On success the node's up state is flipped to upCopied.
func (nt *ovlNodes) copyUp(p string, fn func() error) error {
	_, err, _ := nt.cow.Do(p, func() (any, error) {
		if err := fn(); err != nil {
			return nil, err
		}
		n := nt.get(p)
		n.mu.Lock()
		n.up = upCopied
		n.mu.Unlock()
		return nil, nil
	})
	return err
}

// incHandle interns p (an open anchors a node, decision 15) and increments its
// live-handle count.
func (nt *ovlNodes) incHandle(p string) {
	n := nt.get(p)
	n.mu.Lock()
	n.handles++
	n.mu.Unlock()
}

// decHandle decrements p's live-handle count if the node exists, never going
// below zero.
func (nt *ovlNodes) decHandle(p string) {
	n, ok := nt.peek(p)
	if !ok {
		return
	}
	n.mu.Lock()
	if n.handles > 0 {
		n.handles--
	}
	n.mu.Unlock()
}

// handleCount returns p's current live-handle count (0 if the node is absent).
func (nt *ovlNodes) handleCount(p string) int {
	n, ok := nt.peek(p)
	if !ok {
		return 0
	}
	n.mu.RLock()
	c := n.handles
	n.mu.RUnlock()
	return c
}

// gc evicts the node for p from the table if it carries no masking state and
// has no live handles (decision 16). Copy-up state alone is re-derivable from
// top.Lstat, so a pure copy-up/cache node is reclaimable; whiteout, opaque, and
// open nodes are always retained. The eviction decision is re-checked under the
// node lock and the table lock to stay safe against concurrent mutation.
func (nt *ovlNodes) gc(p string) {
	n, ok := nt.peek(p)
	if !ok {
		return
	}
	n.mu.RLock()
	retain := n.whiteout || n.opaque || n.handles > 0
	n.mu.RUnlock()
	if retain {
		return
	}
	nt.mu.Lock()
	defer nt.mu.Unlock()
	// Re-fetch under the table lock; the node may have been replaced.
	cur, ok := nt.byPath[p]
	if !ok || cur != n {
		return
	}
	// Re-check the node state under its own lock to close the race with a
	// concurrent setWhiteout/incHandle that ran after the RUnlock above.
	cur.mu.RLock()
	stillEvictable := !cur.whiteout && !cur.opaque && cur.handles == 0
	cur.mu.RUnlock()
	if stillEvictable {
		delete(nt.byPath, p)
	}
}

// pathChain returns p and each of its ancestors from the topmost component down
// to p itself: pathChain("a/b/c") == ["a", "a/b", "a/b/c"]. The root "." yields
// an empty chain (it can carry neither whiteout nor opaque as an ancestor key).
func pathChain(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	var (
		out  []string
		acc  string
		rest = p
	)
	for rest != "" {
		i := indexByte(rest, '/')
		var comp string
		if i < 0 {
			comp = rest
			rest = ""
		} else {
			comp = rest[:i]
			rest = rest[i+1:]
		}
		if comp == "" {
			continue
		}
		if acc == "" {
			acc = comp
		} else {
			acc = path.Join(acc, comp)
		}
		out = append(out, acc)
	}
	return out
}

// indexByte is strings.IndexByte without importing strings (keeps the import
// list minimal); it returns the index of the first c in s, or -1.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
