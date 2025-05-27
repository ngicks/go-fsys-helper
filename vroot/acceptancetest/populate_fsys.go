package acceptancetest

import (
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// populates writable fsys using [RootFsys].
func populateRoot(t *testing.T, fsys vroot.Fs) {
	var seq iter.Seq[string] = func(yield func(string) bool) {
		for _, txt := range RootFsys {
			var ok bool
			txt, ok = strings.CutPrefix(txt, "root/readable/")
			if !ok {
				continue
			}
			if !yield(txt) {
				return
			}
		}
	}
	parseAndWriteFsys(t, fsys, seq)
}

func parseAndWriteFsys(t *testing.T, fsys vroot.Fs, seq iter.Seq[string]) {
	for txt := range seq {
		switch {
		case strings.HasSuffix(txt, "/"):
			err := fsys.Mkdir(filepath.FromSlash(txt), fs.ModePerm)
			if err != nil && !errors.Is(err, fs.ErrExist) {
				t.Fatalf("mkdir %q failed with %v", txt, err)
			}
		case strings.Contains(txt, ": "):
			idx := strings.Index(txt, ": ")
			path := txt[:idx]
			content := txt[idx+len(": "):]
			f, err := fsys.OpenFile(filepath.FromSlash(path), os.O_CREATE|os.O_RDWR, fs.ModePerm)
			if err != nil {
				t.Fatalf("open %q failed with %v", path, err)
			}
			_, err = f.Write([]byte(content))
			f.Close()
			if err != nil {
				t.Fatalf("write %q failed with %v", path, err)
			}
		case strings.Contains(txt, " -> "):
			idx := strings.Index(txt, " -> ")
			path := txt[:idx]
			target := txt[idx+len(" -> "):]
			err := fsys.Symlink(filepath.FromSlash(target), filepath.FromSlash(path))
			if err != nil {
				t.Fatalf("symlink %q -> %q failed with %v", path, target, err)
			}
		}
	}
}
