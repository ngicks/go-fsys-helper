package tarfs

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"io/fs"
	"os"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
)

func ungzip(bin []byte) []byte {
	var err error
	gr, err := gzip.NewReader(bytes.NewReader(bin))
	if err != nil {
		panic(err)
	}
	expanded, err := io.ReadAll(gr)
	if err != nil {
		panic(err)
	}
	err = gr.Close()
	if err != nil {
		panic(err)
	}
	return expanded
}

//go:embed testdata/muh/tree.tar.gz
var treeBinGz []byte

var treeBin = ungzip(treeBinGz)

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
			// override mode for windows.
			if runtime.GOOS == "windows" {
				expectedStat = &modeMaskFileInfo{expectedStat}
				actualStat = &modeMaskFileInfo{actualStat}
			}

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
			// Git doesn't preserve mtime on fresh clone, so mask timestamps for comparison
			expectedStat = &timeMaskFileInfo{expectedStat}
			actualStat = &timeMaskFileInfo{actualStat}
			es = fs.FormatFileInfo(expectedStat)
			as = fs.FormatFileInfo(actualStat)
			if es == as {
				return
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

var _ fs.FileInfo = (*timeMaskFileInfo)(nil)

type timeMaskFileInfo struct {
	fs.FileInfo
}

func (t *timeMaskFileInfo) ModTime() time.Time {
	return time.Time{} // Return zero time to mask timestamp differences
}

type modeMaskFileInfo struct {
	fs.FileInfo
}

func (m *modeMaskFileInfo) Mode() fs.FileMode {
	// mode is basically emulated on windows.
	// Forget about detail.
	// Just let it pass.
	// For more detailed testing fs_link_test should be used (wait until go 1.25rc1).
	mode := m.FileInfo.Mode()
	if mode.IsDir() {
		return (mode &^ fs.ModePerm) | (mode & 0o700) | 0o055
	}
	return (mode & 0o700) | 0o044
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
