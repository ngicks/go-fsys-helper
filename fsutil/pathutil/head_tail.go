package pathutil

import (
	"iter"
	"path/filepath"
	"strings"
)

func PathFromHead(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		cut := ""
		vol := filepath.VolumeName(name)
		name := filepath.Clean(name[len(vol):])
		rest := name
		for len(rest) > 0 {
			i := strings.Index(rest, string(filepath.Separator))
			if i < 0 {
				yield(vol + name)
				return
			}
			if i == 0 {
				if !yield(vol + string(filepath.Separator)) {
					return
				}
			} else {
				cut = name[:len(cut)+i]
				if !yield(vol + cut) {
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
		vol := filepath.VolumeName(name)
		name := filepath.Clean(name[len(vol):])
		if !yield(vol + name) {
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
				if !yield(vol + string(filepath.Separator)) {
					return
				}
				break
			} else {
				if !yield(vol + rest) {
					return
				}
			}
		}
	}
}
