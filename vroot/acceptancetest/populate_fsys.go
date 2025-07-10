package acceptancetest

import (
	"iter"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// populates writable fsys using [RootFsys].
func populateRoot(t *testing.T, fsys vroot.Fs) {
	var seq iter.Seq[testhelper.LineDirection] = func(yield func(testhelper.LineDirection) bool) {
		for _, txt := range RootFsys {
			var ok bool
			txt, ok = strings.CutPrefix(txt, "root/readable/")
			if !ok {
				continue
			}
			if !yield(testhelper.ParseLine(txt)) {
				return
			}
		}
	}
	err := testhelper.ExecuteAllLineDirection(fsys, seq)
	if err != nil {
		t.Fatalf("failed to populate fsys: %v", err)
	}
}
