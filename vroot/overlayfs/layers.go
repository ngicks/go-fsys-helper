package overlayfs

import (
	"errors"
	"io/fs"
	"slices"
	"syscall"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// This file provides the RAW stacked-lower view: the mechanical union of the
// read-only lower layers with NO whiteout/opaque masking applied. Masking is
// the overlay's responsibility (it owns the ovl-node table and consults the
// MetadataStore); these helpers only answer "what do the lowers, by
// themselves, say about this path?".
//
// Lowers are ordered low→high priority: the LAST element of the slice is the
// highest priority. Resolution therefore walks top→lowers[n-1]→…→lowers[0]
// (iterate with slices.Backward). Symlinks are NOT followed here — the
// overlay's resolve does that across the merged view.
//
// These signatures are consumed by the overlay (a later wave) and that wave
// may extend this file.

// lowerLstat returns the [fs.FileInfo] of name from the highest-priority lower
// that has it (via Lstat, so symlinks are not followed), together with the
// winning layer's fsys. It walks lowers high→low priority and returns the
// first hit. If no lower has name it returns fs.ErrNotExist.
//
// Errors other than fs.ErrNotExist from a lower are returned immediately
// (a lower failing for a reason other than "absent" is not silently skipped).
func lowerLstat(
	lowers []Layer,
	name string,
) (fsys vroot.Fs[vroot.File], info fs.FileInfo, err error) {
	for _, l := range slices.Backward(lowers) {
		info, err = l.fsys.Lstat(name)
		if err == nil {
			return l.fsys, info, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	return nil, nil, fs.ErrNotExist
}

// lowerReadDir returns the merged dirents of directory dir across all lowers
// that have it, as a name→[fs.DirEntry] map. Entries are deduped by name with
// the higher-priority lower winning. Lowers where dir is absent or is not a
// directory are skipped (their absence simply contributes nothing to the
// union); any other error reading a lower is returned.
//
// No whiteout/opaque filtering is applied and symlinks are not followed; the
// overlay later overlays the top entries onto this map and applies whiteout /
// opaque masking before yielding a listing.
func lowerReadDir(lowers []Layer, dir string) (map[string]fs.DirEntry, error) {
	merged := make(map[string]fs.DirEntry)
	// Walk low→high so that higher-priority lowers overwrite lower-priority
	// entries of the same name; the last writer (highest priority) wins.
	for _, l := range lowers {
		dirents, err := vroot.ReadDir(l.fsys, dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
				continue
			}
			return nil, err
		}
		for _, de := range dirents {
			merged[de.Name()] = de
		}
	}
	return merged, nil
}

// isNotDir reports whether err indicates the target is not a directory
// (ENOTDIR), which lowerReadDir treats like absence.
func isNotDir(err error) bool {
	return errors.Is(err, syscall.ENOTDIR)
}
