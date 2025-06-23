package fsutil

import (
	"iter"
	"path/filepath"
	"strings"
)

type pathComponent struct {
	component   string
	offsetStart int
	offsetEnd   int
}

func splitPathSeq(path string) iter.Seq2[int, pathComponent] {
	return func(yield func(int, pathComponent) bool) {
		i := 0
		off := 0
		offNext := 0
		for s := range strings.SplitSeq(path, string(filepath.Separator)) {
			offNext += len(s) + 1
			if !yield(i, pathComponent{s, off, offNext}) {
				return
			}
			i++
			off = offNext
		}
	}
}
