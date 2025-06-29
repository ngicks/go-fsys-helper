package paths

import (
	"iter"
	"path/filepath"
	"strings"
)

func PathFromHead(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		cut := ""
		name := filepath.Clean(name)
		rest := name
		for len(rest) > 0 {
			i := strings.Index(rest, string(filepath.Separator))
			if i < 0 {
				yield(name)
				return
			}
			if i == 0 {
				if !yield(string(filepath.Separator)) {
					return
				}
			} else {
				cut = name[:len(cut)+i]
				if !yield(cut) {
					return
				}
			}
			cut = name[:len(cut)+1] // include last sep
			rest = rest[i+len(string(filepath.Separator)):]
		}
	}
}

func PathFromTail(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		name := filepath.Clean(name)
		if !yield(name) {
			return
		}
		if name == "." {
			return
		}
		rest := name
		for len(rest) > 0 {
			i := strings.LastIndex(rest, string(filepath.Separator))
			if i < 0 {
				return
			}
			rest = rest[:i]
			if i == 0 {
				if !yield(string(filepath.Separator)) {
					return
				}
				break
			} else {
				if !yield(rest) {
					return
				}
			}
		}
	}
}
