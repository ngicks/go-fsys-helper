package tarfs

import (
	"bytes"
	_ "embed"
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
	fsys, err := New(bytes.NewReader(treeBin), nil)
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
		compareStat(t, dirFs, fsys, path)
		if !d.IsDir() {
			compareContent(t, dirFs, fsys, path)
		}
		return nil
	})

	// skip '.' since TestFS fails stating it did not find '.' in the fsys.
	if err := fstest.TestFS(fsys, seen[1:]...); err != nil {
		t.Errorf("fstest.TestFS fail: %v", err)
	}
}

func compareStat(t *testing.T, expected, actual fs.FS, path string) (expectedStat, actualStat fs.FileInfo) {
	var err error
	expectedStat, err = fs.Stat(expected, path)
	if err != nil {
		panic(err)
	}
	actualStat, err = fs.Stat(actual, path)
	if err != nil {
		panic(err)
	}

	{
		es := fs.FormatFileInfo(expectedStat)
		as := fs.FormatFileInfo(actualStat)
		if es != as {
			if expectedStat.IsDir() && actualStat.IsDir() {
				// tar returns dir as size 0, while usual linux filesystem returns it as 4KiB.
				expectedStat = &sizeMaskFileInfo{expectedStat}
				actualStat = &sizeMaskFileInfo{actualStat}
				es = fs.FormatFileInfo(expectedStat)
				as = fs.FormatFileInfo(actualStat)
				if es == as {
					return
				}
			}
			t.Errorf("stat not equal: expected(%q) != atcual(%q)", es, as)
			return
		}
	}
	return
}

var _ fs.FileInfo = (*sizeMaskFileInfo)(nil)

type sizeMaskFileInfo struct {
	fs.FileInfo
}

func (s *sizeMaskFileInfo) Size() int64 {
	return 0
}

func compareContent(t *testing.T, expected, actual fs.FS, path string) {
	binExpected, err := fs.ReadFile(expected, path)
	if err != nil {
		panic(err)
	}
	binActual, err := fs.ReadFile(actual, path)
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(binExpected, binActual) {
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
}
