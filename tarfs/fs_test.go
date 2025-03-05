package tarfs

import (
	"bytes"
	_ "embed"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

var (
	//go:embed testdata/muh/tree.tar
	treeBin []byte
)

func TestFs(t *testing.T) {
	fsys, err := New(bytes.NewReader(treeBin))
	if err != nil {
		panic(err)
	}

	dirFs := os.DirFS("testdata/muh/tree")
	var seen []string
	_ = fs.WalkDir(dirFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		seen = append(seen, path)

		f, err := fsys.Open(path)
		if err != nil {
			t.Errorf("open %q: %v", path, err)
			return err
		}
		defer f.Close()
		s, err := f.Stat()
		if err != nil {
			t.Errorf("stat %q: %v", path, err)
			return err
		}
		if s.IsDir() != d.IsDir() {
			t.Errorf("wrongly is dir: %q", path)
			return err
		}
		if s.IsDir() {
			return nil
		}
		binActual, err := io.ReadAll(f)
		if err != nil {
			t.Errorf("read %q: %v", path, err)
			return err
		}
		fExpected, err := dirFs.Open(path)
		if err != nil {
			panic(err)
		}
		defer fExpected.Close()
		binExpected, err := io.ReadAll(fExpected)
		if err != nil {
			panic(err)
		}
		if !bytes.Equal(binActual, binExpected) {
			t.Errorf(
				`read content not equal
filename = %q
expected = %q(%d)
actual = %q(%d)

`,
				path,
				ellipsis(binExpected), len(binExpected),
				ellipsis(binActual), len(binActual),
			)
		}
		return nil
	})

	if err := fstest.TestFS(fsys, seen[1:]...); err != nil {
		t.Errorf("fstest.TestFS fail: %v", err)
	}
}
