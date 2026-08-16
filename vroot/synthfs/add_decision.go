package synthfs

import "io/fs"

// AddDecision is the outcome chosen by an [AddFunc] for a single source entry.
type AddDecision int

const (
	// AddDecisionSkip drops the incoming entry. For a directory its whole
	// subtree is pruned (its children are not visited). Any existing entry at
	// the path is left untouched.
	AddDecisionSkip AddDecision = iota
	// AddDecisionKeep prefers the existing entry. For a directory it MERGES:
	// the directory is reused (created if absent) and the source's children
	// are visited so they combine with it. For a non-directory the existing
	// leaf is kept and the incoming dropped (if no existing entry, the
	// incoming is also not added).
	AddDecisionKeep
	// AddDecisionOverride replaces whatever is at the path with the incoming
	// entry. For a directory, the source's children are placed under the
	// fresh dir.
	AddDecisionOverride
)

func (a AddDecision) String() string {
	switch a {
	case AddDecisionSkip:
		return "Skip"
	case AddDecisionKeep:
		return "Keep"
	case AddDecisionOverride:
		return "Override"
	}
	return "Unknown"
}

// AddEntry describes a single source or existing tree entry for an [AddFunc].
type AddEntry interface {
	// Path is the slash-separated path within the synth tree where the entry
	// would land. Always reported in slash form regardless of host OS.
	Path() string
	// Info is the pre-resolved metadata for the entry (mode/size/mtime).
	Info() fs.FileInfo
}

// AddFunc decides what to do with a single source entry as it is being
// ingested.
//
// incoming is never nil; it describes the source entry under consideration.
// existing is the entry already at the same path, or nil if the path is free.
//
// A nil error commits the chosen [AddDecision]. A non-nil error aborts the
// whole [Root.AddFile] / [Root.AddFS] call and propagates as its return value.
//
// Passing nil for the AddFunc parameter of AddFile/AddFS uses
// [MergeOverwrite].
type AddFunc func(incoming, existing AddEntry) (AddDecision, error)

// MergeOverwrite is the default resolver: directories merge (recurse), and
// leaf entries (file/symlink) overwrite whatever is at the path.
func MergeOverwrite(incoming, existing AddEntry) (AddDecision, error) {
	if existing == nil {
		return AddDecisionOverride, nil
	}
	if incoming.Info().IsDir() && existing.Info().IsDir() {
		return AddDecisionKeep, nil
	}
	return AddDecisionOverride, nil
}

// MergeKeep prefers existing entries; directories still merge but leaf
// conflicts keep the existing leaf and drop the incoming.
func MergeKeep(incoming, existing AddEntry) (AddDecision, error) {
	if existing == nil {
		return AddDecisionOverride, nil
	}
	// Both dir-on-dir and any-on-leaf collapse to Keep here; the recursion
	// distinguishes them upstream.
	return AddDecisionKeep, nil
}

// FailOnConflict merges directories but errors if a leaf entry already
// exists at the target path.
func FailOnConflict(incoming, existing AddEntry) (AddDecision, error) {
	if existing == nil {
		return AddDecisionOverride, nil
	}
	if incoming.Info().IsDir() && existing.Info().IsDir() {
		return AddDecisionKeep, nil
	}
	return AddDecisionSkip, &fs.PathError{
		Op:   "add",
		Path: incoming.Path(),
		Err:  fs.ErrExist,
	}
}

// addEntry is the concrete [AddEntry] used by synthfs internally.
type addEntry struct {
	p    string
	info fs.FileInfo
}

func (e *addEntry) Path() string      { return e.p }
func (e *addEntry) Info() fs.FileInfo { return e.info }
