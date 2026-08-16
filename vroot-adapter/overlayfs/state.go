package overlayfs

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// overlayState is the overlay's masking state and the merged view computed from
// it: which layer backs a name, and what a directory holds once every layer has
// had its say.
//
// Layers are indexed in mount order with the top at 0: layers[0] is the top,
// layers[1+i] is lowers[i], and a name resolves 0 → 1 → … (D20). Masking
// carries the index it was recorded at and only ever reaches deeper: a whiteout
// or an opaque mark at layer i hides layers below i, never layer i itself and
// never a layer above it. That is what lets a canonical lower go on masking
// what it was stacked over while the top, and the lowers between them, stay
// visible through it.
//
// Only masking is held in memory; directory content is read from the layers on
// every call. The reference implementation caches a node arena of parents and
// children because FUSE hands it inode numbers it has to map back onto layer
// paths — vroot names files by path, so there is nothing to map and nothing
// worth staling.
//
// # Locking
//
// mu serializes every masking read and write. The caller holds it across a
// whole overlay operation and the methods here take nothing themselves: a
// lookup, the copy-up it decides on and the mask that copy-up records have to
// land as one step, and only the caller knows where that step ends. Queries
// need at least the read lock, mutators the write lock. Data I/O on files
// already opened through the merged view never takes mu (D16).
type overlayState struct {
	mu sync.RWMutex
	// layers and masks are parallel, and neither grows after construction.
	layers []*DataSource
	masks  []layerMask
}

// layerMask is one layer's masking state, keyed the way [MetadataStore] keys
// it — clean slash paths relative to that layer's content root, "" for the root
// — so a mark in memory and the row that persists it are the same string.
type layerMask struct {
	whiteout map[string]struct{}
	opaque   map[string]struct{}
}

// newOverlayState stacks top over lowers, in mount order, and seeds each
// layer's masking from the store that layer owns; a plain layer owns none and
// masks nothing.
//
// top must be canonical: the overlay's own masking has nowhere else to go.
// Whether top's store is writable is [New]'s to judge — a read-only one still
// describes its layer correctly, it only refuses the next change.
func newOverlayState(top *DataSource, lowers []*DataSource) (*overlayState, error) {
	if top == nil || !top.canonical || top.store == nil {
		return nil, fmt.Errorf("overlayfs: %w: top", ErrNotCanonical)
	}
	s := &overlayState{layers: append([]*DataSource{top}, lowers...)}
	s.masks = make([]layerMask, len(s.layers))
	for i, d := range s.layers {
		m := layerMask{
			whiteout: make(map[string]struct{}),
			opaque:   make(map[string]struct{}),
		}
		s.masks[i] = m
		if d.store == nil {
			continue
		}
		whiteouts, opaques, err := d.store.Load()
		if err != nil {
			return nil, err
		}
		for _, name := range whiteouts {
			m.whiteout[cleanName(name)] = struct{}{}
		}
		for _, name := range opaques {
			m.opaque[cleanName(name)] = struct{}{}
		}
	}
	return s, nil
}

// marks reports what layer records for name itself: the state a write has to
// bring in line when it puts content back where a mask stands.
func (s *overlayState) marks(layer int, name string) (whiteout bool, opaque bool) {
	name = cleanName(name)
	_, whiteout = s.masks[layer].whiteout[name]
	_, opaque = s.masks[layer].opaque[name]
	return whiteout, opaque
}

// setWhiteout masks name in every layer below the top and forgets the marks the
// top recorded under it: nothing under a masked name can be addressed any more,
// and name's own opaque mark goes with them because what replaces the directory
// that carried it may not be a directory. It mirrors
// [MetadataStore.SetWhiteout], whose contract this is.
//
// The store commits first here and in every mutator below. The other order
// would leave the overlay answering from a mask no restart could find.
func (s *overlayState) setWhiteout(name string) error {
	name = cleanName(name)
	if err := s.layers[0].store.SetWhiteout(name); err != nil {
		return err
	}
	s.masks[0].whiteout[name] = struct{}{}
	delete(s.masks[0].opaque, name)
	s.masks[0].dropSubtree(name)
	return nil
}

// clearWhiteout unmasks name, leaving its opaque mark alone.
func (s *overlayState) clearWhiteout(name string) error {
	name = cleanName(name)
	if err := s.layers[0].store.ClearWhiteout(name); err != nil {
		return err
	}
	delete(s.masks[0].whiteout, name)
	return nil
}

// setOpaque marks dir opaque: the layers below the top stop contributing
// children to it. The marks recorded under dir stay — dir is still there, and
// clearing the mark brings the layers below back into view.
func (s *overlayState) setOpaque(dir string) error {
	dir = cleanName(dir)
	if err := s.layers[0].store.SetOpaque(dir); err != nil {
		return err
	}
	s.masks[0].opaque[dir] = struct{}{}
	return nil
}

// clearOpaque lets the layers below the top contribute children to dir again,
// leaving its whiteout alone.
func (s *overlayState) clearOpaque(dir string) error {
	dir = cleanName(dir)
	if err := s.layers[0].store.ClearOpaque(dir); err != nil {
		return err
	}
	delete(s.masks[0].opaque, dir)
	return nil
}

// maskedUnder lists what the top records strictly under dir, shallowest first.
//
// It is the range read [MetadataStore] has no method for: the interface is five
// single-name calls (D18), and a custom store handed to
// [NewDataSourceCanonicalStore] implements only those. The in-memory masks are
// authoritative (D14) and hold every row the store does, so the range is read
// here and replayed onto the store one call at a time.
func (s *overlayState) maskedUnder(dir string) (whiteouts []string, opaques []string) {
	prefix := dir + "/"
	if dir == "" {
		prefix = ""
	}
	under := func(name string) bool { return name != dir && strings.HasPrefix(name, prefix) }
	for name := range s.masks[0].whiteout {
		if under(name) {
			whiteouts = append(whiteouts, name)
		}
	}
	for name := range s.masks[0].opaque {
		if under(name) {
			opaques = append(opaques, name)
		}
	}
	slices.Sort(whiteouts)
	slices.Sort(opaques)
	return whiteouts, opaques
}

// clearSubtree forgets everything the top records at name and under it, which is
// what a removal that needs no whiteout leaves behind: the tree those marks
// described is gone, and a mark on a name nothing holds would mask a layer below
// that a later create is entitled to see again.
//
// [overlayState.setWhiteout] does the same pruning in one store call; this is
// the branch where no whiteout is recorded and so nothing prunes for us.
func (s *overlayState) clearSubtree(name string) error {
	name = cleanName(name)
	whiteouts, opaques := s.maskedUnder(name)
	for _, n := range append(whiteouts, name) {
		if _, ok := s.masks[0].whiteout[n]; !ok {
			continue
		}
		if err := s.clearWhiteout(n); err != nil {
			return err
		}
	}
	for _, n := range append(opaques, name) {
		if _, ok := s.masks[0].opaque[n]; !ok {
			continue
		}
		if err := s.clearOpaque(n); err != nil {
			return err
		}
	}
	return nil
}

// renameSubtree moves what the top records strictly under oldDir to the matching
// names under newDir. The marks on oldDir itself are left alone: a rename decides
// those from what the layers below still show at either end, which this cannot
// see.
//
// The reference gets this for free — its whiteouts are files inside the directory
// being renamed, so one renameat carries them along (C main.c:4864 @ 4759abd).
// Rows keyed by full path (D19) do not move with the tree, so they are moved
// here, composed from the four single-name mutators the store contract offers.
//
// The new marks go on before the old ones come off. A store failure part-way
// then leaves the subtree masked at both ends rather than at neither: masking a
// name twice hides content that is there, masking it nowhere shows content that
// was meant to be gone, and only the second is a wrong answer the caller cannot
// tell from a right one.
//
// Within the set pass, whiteouts are replayed before opaques and each kind
// shallowest first, so a whiteout landing on an ancestor of a mark still to come
// prunes only what the store contract says it should — nothing is recorded
// strictly under a whiteout to begin with, [MetadataStore.SetWhiteout] dropping
// that subtree. The clear pass that follows takes none of the set pass back, and
// neither does that same subtree drop, the two trees being disjoint: [Fs.rename]
// refuses a destination equal to the source or inside it, and
// [Fs.renameTargetCheck] refuses one the merged view still shows any entry in.
func (s *overlayState) renameSubtree(oldDir string, newDir string) error {
	oldDir, newDir = cleanName(oldDir), cleanName(newDir)
	whiteouts, opaques := s.maskedUnder(oldDir)
	if len(whiteouts) == 0 && len(opaques) == 0 {
		return nil
	}
	rebase := func(n string) string {
		return childName(newDir, strings.TrimPrefix(strings.TrimPrefix(n, oldDir), "/"))
	}
	for _, n := range whiteouts {
		if err := s.setWhiteout(rebase(n)); err != nil {
			return err
		}
	}
	for _, n := range opaques {
		if err := s.setOpaque(rebase(n)); err != nil {
			return err
		}
	}
	for _, n := range whiteouts {
		if err := s.clearWhiteout(n); err != nil {
			return err
		}
	}
	for _, n := range opaques {
		if err := s.clearOpaque(n); err != nil {
			return err
		}
	}
	return nil
}

// cut returns the deepest layer that can still show name. Masking reaches only
// deeper than the layer that recorded it, so the shallowest layer masking name
// bounds the search there. A whiteout counts on name itself and on every
// ancestor; an opaque mark counts on ancestors only — it hides a directory's
// lower children, not the directory.
func (s *overlayState) cut(name string) int {
	return s.cutFrom(name, 0)
}

// cutFrom is [overlayState.cut] counting only what the layers at or below from
// record. It is the bound of a search those layers alone answer, and the masking
// above them is no part of it: [overlayState.lookupFrom] asks what would still be
// there if the layers above gave name up, which they cannot be said to have done
// while their own marks go on hiding it.
//
// A whiteout the top holds on the very name a removal is about to drop would
// otherwise bound the probe at the top and answer that no layer below shows the
// name — and the removal, believing nothing is left to mask, would clear that
// whiteout and let the lower entry back. The two can stand together on one name
// after a removal that recorded its mask and then failed to drop the file.
func (s *overlayState) cutFrom(name string, from int) int {
	cut := len(s.layers) - 1
	for n, self := name, true; ; n, self = parentName(n), false {
		for i := from; i <= cut; i++ {
			m := s.masks[i]
			_, whiteout := m.whiteout[n]
			_, opaque := m.opaque[n]
			if whiteout || (opaque && !self) {
				cut = i
				break
			}
		}
		if n == "" {
			return cut
		}
	}
}

// childCut returns the deepest layer that can still contribute children to the
// directory name: whichever is shallower of what bounds name itself and the
// layer that marked it opaque.
func (s *overlayState) childCut(name string) int {
	cut := s.cut(name)
	for i, m := range s.masks[:cut+1] {
		if _, ok := m.opaque[name]; ok {
			return i
		}
	}
	return cut
}

// whiteoutAbove reports whether a layer above i whited out name. It stands in
// for the whole of [overlayState.cut] on a merged directory's entry: the
// ancestors cut would walk are that directory and up, which the caller already
// bounded its layer range by.
func (s *overlayState) whiteoutAbove(i int, name string) bool {
	for _, m := range s.masks[:i] {
		if _, ok := m.whiteout[name]; ok {
			return true
		}
	}
	return false
}

// layerPath maps a merged-view name onto its path in layer i's content tree,
// spelled with the separator the backend expects rather than the "/" the
// masking state is keyed by.
func (s *overlayState) layerPath(i int, name string) string {
	return s.layers[i].ContentPath(name)
}

// dropSubtree forgets every mark recorded strictly under name, the range delete
// [MetadataStore.SetWhiteout] runs in memory. It scans the layer's marks rather
// than walking a children index: a layer holds few of them, and the store
// commit the caller just waited for costs more than the scan.
func (m layerMask) dropSubtree(name string) {
	prefix := name + "/"
	if name == "" {
		prefix = ""
	}
	under := func(n string, _ struct{}) bool { return n != name && strings.HasPrefix(n, prefix) }
	maps.DeleteFunc(m.whiteout, under)
	maps.DeleteFunc(m.opaque, under)
}

// childName joins a directory and one of its entries in the spelling the
// masking state is keyed by. Either half may be "" — the root, or the directory
// itself — and the other is then the whole answer.
func childName(dir string, name string) string {
	switch {
	case dir == "":
		return name
	case name == "":
		return dir
	}
	return dir + "/" + name
}
