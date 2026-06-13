// Package paths provides iterators over path components, allowing callers
// to walk a path from its head (outermost) or tail (innermost) segment.
//
// Both iterators clean their input with [filepath.Clean] before walking, so they
// never emit an empty token (from a leading or doubled separator) nor a spurious
// ".." token introduced by an uncleaned input such as "a/../b" (which cleans to
// "b"). A genuinely upward path such as "../a" keeps its leading ".." after
// cleaning and that component is emitted as-is — Clean cannot resolve it.
//
// For an absolute path the bare filesystem root ("/" on unix) is not emitted as
// its own token: the iterators yield the first named component onward
// ("/a", "/a/b", …), since the root is never a component a caller would create
// or compare. The current-directory path "." yields nothing (it has no named
// components).
package paths

import (
	"iter"
	"path/filepath"
	"strings"
)

// PathFromHead yields progressively longer prefixes of name, from the outermost
// component to the whole (cleaned) path: PathFromHead("a/b/c") yields "a",
// "a/b", "a/b/c". See the package doc for input cleaning and absolute/"." rules.
func PathFromHead(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		name = filepath.Clean(name)
		if name == "." {
			return
		}
		sep := filepath.Separator
		// For an absolute path, start scanning past the leading separator so the
		// first emitted prefix is the root + first component (e.g. "/a"), never a
		// bare "" or "/".
		start := 0
		if filepath.IsAbs(name) {
			// Skip the volume name (Windows) and the leading separator.
			vol := filepath.VolumeName(name)
			start = len(vol)
			for start < len(name) && name[start] == byte(sep) {
				start++
			}
		}
		if start >= len(name) {
			// Bare root (e.g. "/"): no named components to emit.
			return
		}
		for i := start; i < len(name); i++ {
			if name[i] == byte(sep) {
				if !yield(name[:i]) {
					return
				}
			}
		}
		yield(name)
	}
}

// PathFromTail yields progressively shorter prefixes of name, from the whole
// (cleaned) path down to its outermost component: PathFromTail("a/b/c") yields
// "a/b/c", "a/b", "a". See the package doc for input cleaning and absolute/"."
// rules.
func PathFromTail(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		name = filepath.Clean(name)
		if name == "." {
			return
		}
		sep := string(filepath.Separator)
		// The lowest prefix worth yielding for an absolute path is the root +
		// first component (e.g. "/a"); stop before the bare root.
		var floor int
		if filepath.IsAbs(name) {
			vol := filepath.VolumeName(name)
			floor = len(vol)
			for floor < len(name) && name[floor] == byte(filepath.Separator) {
				floor++
			}
		}
		if len(name) <= floor {
			// Bare root (e.g. "/"): no named components to emit.
			return
		}
		if !yield(name) {
			return
		}
		rest := name
		for len(rest) > floor {
			i := strings.LastIndex(rest, sep)
			if i < 0 || i < floor {
				return
			}
			rest = rest[:i]
			if len(rest) < floor {
				return
			}
			if !yield(rest) {
				return
			}
		}
	}
}
