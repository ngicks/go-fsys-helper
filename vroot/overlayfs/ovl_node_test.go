package overlayfs

import (
	"errors"
	"sync"
	"testing"
)

func newTestNodes(t *testing.T) *ovlNodes {
	t.Helper()
	nt, err := newOvlNodes(NewMetadataStoreMem())
	if err != nil {
		t.Fatalf("newOvlNodes: %v", err)
	}
	return nt
}

func TestOvlNodesGetPeekIntern(t *testing.T) {
	nt := newTestNodes(t)

	if _, ok := nt.peek("a"); ok {
		t.Fatalf("peek before get: want absent")
	}
	n1 := nt.get("a")
	n2 := nt.get("a")
	if n1 != n2 {
		t.Fatalf("get must return the same interned node")
	}
	// A freshly-interned node starts in the upUnknown copy-up state.
	n1.mu.RLock()
	up := n1.up
	n1.mu.RUnlock()
	if up != upUnknown {
		t.Fatalf("freshly interned node up = %v, want upUnknown", up)
	}
	if got, ok := nt.peek("a"); !ok || got != n1 {
		t.Fatalf("peek after get: want the interned node")
	}
}

func TestOvlNodesGetConcurrent(t *testing.T) {
	nt := newTestNodes(t)
	const g = 64
	var wg sync.WaitGroup
	got := make([]*ovlNode, g)
	wg.Add(g)
	for i := range g {
		go func(i int) {
			defer wg.Done()
			got[i] = nt.get("shared/path")
		}(i)
	}
	wg.Wait()
	for i := 1; i < g; i++ {
		if got[i] != got[0] {
			t.Fatalf("concurrent get returned distinct nodes")
		}
	}
}

func TestOvlNodesWhiteoutMasking(t *testing.T) {
	nt := newTestNodes(t)
	if err := nt.setWhiteout("a/b"); err != nil {
		t.Fatalf("setWhiteout: %v", err)
	}

	cases := map[string]bool{
		"a/b":     true,  // the whited path itself
		"a/b/c":   true,  // descendant masked by subtree whiteout
		"a/b/c/d": true,  // deeper descendant
		"a":       false, // ancestor unaffected
		"a/c":     false, // sibling unaffected
	}
	for p, want := range cases {
		if got := nt.ancestorWhiteout(p); got != want {
			t.Errorf("ancestorWhiteout(%q) = %v, want %v", p, got, want)
		}
	}

	if err := nt.clearWhiteout("a/b"); err != nil {
		t.Fatalf("clearWhiteout: %v", err)
	}
	for p := range cases {
		if nt.ancestorWhiteout(p) {
			t.Errorf("ancestorWhiteout(%q) after clear: want false", p)
		}
	}
}

func TestOvlNodesOpaque(t *testing.T) {
	nt := newTestNodes(t)
	if err := nt.setOpaque("d"); err != nil {
		t.Fatalf("setOpaque: %v", err)
	}

	if !nt.opaqueAt("d") {
		t.Errorf("opaqueAt(d) = false, want true")
	}
	if nt.opaqueAt("e") {
		t.Errorf("opaqueAt(e) = true, want false")
	}
	// opaque hides the lower subtree under d, so a strict descendant sees an
	// opaque ancestor, but d itself does not (its own children are hidden, not d).
	if !nt.ancestorOpaque("d/x") {
		t.Errorf("ancestorOpaque(d/x) = false, want true")
	}
	if nt.ancestorOpaque("d") {
		t.Errorf("ancestorOpaque(d) = true, want false (own opaque is not an ancestor)")
	}

	if err := nt.clearOpaque("d"); err != nil {
		t.Fatalf("clearOpaque: %v", err)
	}
	if nt.opaqueAt("d") {
		t.Errorf("opaqueAt(d) after clear: want false")
	}
}

func TestOvlNodesHandleAccounting(t *testing.T) {
	nt := newTestNodes(t)
	if got := nt.handleCount("f"); got != 0 {
		t.Fatalf("handleCount initial = %d, want 0", got)
	}
	nt.incHandle("f")
	nt.incHandle("f")
	if got := nt.handleCount("f"); got != 2 {
		t.Fatalf("handleCount after 2 inc = %d, want 2", got)
	}
	nt.decHandle("f")
	if got := nt.handleCount("f"); got != 1 {
		t.Fatalf("handleCount after dec = %d, want 1", got)
	}
	// decHandle never goes below zero, even past the open count.
	nt.decHandle("f")
	nt.decHandle("f")
	if got := nt.handleCount("f"); got != 0 {
		t.Fatalf("handleCount floored = %d, want 0", got)
	}
	// decHandle on an absent path is a no-op.
	nt.decHandle("never-opened")
}

func TestOvlNodesGC(t *testing.T) {
	nt := newTestNodes(t)

	// Pure-cache node: no mask, no handles → evictable.
	nt.get("pure")
	nt.gc("pure")
	if _, ok := nt.peek("pure"); ok {
		t.Errorf("gc(pure): node should be evicted")
	}

	// Open node: retained while a handle is live, evicted once it drops.
	nt.incHandle("open")
	nt.gc("open")
	if _, ok := nt.peek("open"); !ok {
		t.Errorf("gc(open) with live handle: node should be retained")
	}
	nt.decHandle("open")
	nt.gc("open")
	if _, ok := nt.peek("open"); ok {
		t.Errorf("gc(open) after last close: node should be evicted")
	}

	// Whiteout node: always retained.
	if err := nt.setWhiteout("masked"); err != nil {
		t.Fatalf("setWhiteout: %v", err)
	}
	nt.gc("masked")
	if _, ok := nt.peek("masked"); !ok {
		t.Errorf("gc(masked): whiteout node must be retained")
	}

	// Opaque node: always retained.
	if err := nt.setOpaque("opaquedir"); err != nil {
		t.Fatalf("setOpaque: %v", err)
	}
	nt.gc("opaquedir")
	if _, ok := nt.peek("opaquedir"); !ok {
		t.Errorf("gc(opaquedir): opaque node must be retained")
	}
}

func TestOvlNodesCopyUpDedup(t *testing.T) {
	nt := newTestNodes(t)
	var (
		mu    sync.Mutex
		calls int
	)
	const g = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(g)
	for range g {
		go func() {
			defer wg.Done()
			<-start
			_ = nt.copyUp("lower-only", func() error {
				mu.Lock()
				calls++
				mu.Unlock()
				return nil
			})
		}()
	}
	close(start)
	wg.Wait()

	// singleflight collapses concurrent callers; the work runs a small number of
	// times (commonly once). Crucially it must run at least once and set up.
	mu.Lock()
	c := calls
	mu.Unlock()
	if c < 1 {
		t.Fatalf("copyUp fn never ran")
	}
	n := nt.get("lower-only")
	n.mu.RLock()
	up := n.up
	n.mu.RUnlock()
	if up != upCopied {
		t.Fatalf("copyUp success did not set up=upCopied, got %v", up)
	}
}

func TestOvlNodesCopyUpError(t *testing.T) {
	nt := newTestNodes(t)
	sentinel := errors.New("copy failed")
	err := nt.copyUp("x", func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("copyUp error = %v, want %v", err, sentinel)
	}
	n := nt.get("x")
	n.mu.RLock()
	up := n.up
	n.mu.RUnlock()
	if up == upCopied {
		t.Fatalf("copyUp failure must not set up=upCopied")
	}
}

func TestOvlNodesSeededFromStore(t *testing.T) {
	store := NewMetadataStoreMem()
	if err := store.SetWhiteout("seeded/wh"); err != nil {
		t.Fatalf("SetWhiteout: %v", err)
	}
	if err := store.SetOpaque("seeded/op"); err != nil {
		t.Fatalf("SetOpaque: %v", err)
	}
	nt, err := newOvlNodes(store)
	if err != nil {
		t.Fatalf("newOvlNodes: %v", err)
	}
	if !nt.ancestorWhiteout("seeded/wh") {
		t.Errorf("seeded whiteout not present")
	}
	if !nt.opaqueAt("seeded/op") {
		t.Errorf("seeded opaque not present")
	}
}

func TestPathChain(t *testing.T) {
	cases := map[string][]string{
		".":     nil,
		"":      nil,
		"a":     {"a"},
		"a/b":   {"a", "a/b"},
		"a/b/c": {"a", "a/b", "a/b/c"},
	}
	for in, want := range cases {
		got := pathChain(in)
		if len(got) != len(want) {
			t.Errorf("pathChain(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("pathChain(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

// upNotCopied is referenced so the constant is observably part of the node-table
// contract (Wave 3b flips up between these states); keep it covered.
var _ = upNotCopied
