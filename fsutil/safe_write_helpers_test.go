package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

type (
	testSafeWriteOption      = SafeWriteOption[*osfslite.OsfsLite, *os.File]
	testTempFilePolicyRandom = TempFilePolicyRandom[*osfslite.OsfsLite, *os.File]
	testTempFilePolicyDir    = TempFilePolicyDir[*osfslite.OsfsLite, *os.File]
)

var (
	newTestTempFilePolicyRandom = NewTempFilePolicyRandom[*osfslite.OsfsLite]
	newTestTempFilePolicyDir    = NewTempFilePolicyDir[*osfslite.OsfsLite]
)

// testFsysWrapper wraps osfsLite to implement fs.FS for fs.WalkDir
type testFsysWrapper struct {
	fsys *osfslite.OsfsLite
}

func (f *testFsysWrapper) Open(name string) (fs.File, error) {
	file, err := f.fsys.OpenFile(filepath.FromSlash(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// errorReader is a reader that always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

type mockDirEntry struct {
	isDir bool
}

func (m *mockDirEntry) Name() string               { return "mock" }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }
