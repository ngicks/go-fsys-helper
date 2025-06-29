package fsutil

import (
	"io/fs"
	"os"
	"strings"
)

// mockErrorFs wraps an existing filesystem and injects errors for specific operations
type mockErrorFs struct {
	osfsLite
	mkdirError         error
	mkdirErrorPath     string
	openFileError      error
	openFileErrorPath  string
	chmodError         error
	chmodErrorPath     string
	symlinkError       error
	symlinkErrorTarget string
}

// Create a type alias for testing
type (
	testMockErrorFs      = mockErrorFs
	testMockCopyFsOption = CopyFsOption[*mockErrorFs, *os.File]
)

func (m *mockErrorFs) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	if m.openFileError != nil && (m.openFileErrorPath == "" || strings.Contains(name, m.openFileErrorPath)) {
		return nil, m.openFileError
	}
	return m.osfsLite.OpenFile(name, flag, perm)
}

func (m *mockErrorFs) Mkdir(name string, perm fs.FileMode) error {
	if m.mkdirError != nil && (m.mkdirErrorPath == "" || strings.Contains(name, m.mkdirErrorPath)) {
		return m.mkdirError
	}
	return m.osfsLite.Mkdir(name, perm)
}

func (m *mockErrorFs) Chmod(name string, mode fs.FileMode) error {
	if m.chmodError != nil && (m.chmodErrorPath == "" || strings.Contains(name, m.chmodErrorPath)) {
		return m.chmodError
	}
	return m.osfsLite.Chmod(name, mode)
}

func (m *mockErrorFs) Symlink(oldname, newname string) error {
	if m.symlinkError != nil && (m.symlinkErrorTarget == "" || strings.Contains(newname, m.symlinkErrorTarget)) {
		return m.symlinkError
	}
	return m.osfsLite.Symlink(oldname, newname)
}

func (m *mockErrorFs) ReadLink(name string) (string, error) {
	return m.osfsLite.ReadLink(name)
}

// mockErrorSrcFs wraps a filesystem and injects errors for read operations
type mockErrorSrcFs struct {
	base      fs.FS
	openError error
	openPath  string
}

func (m *mockErrorSrcFs) Open(name string) (fs.File, error) {
	if m.openError != nil && (m.openPath == "" || strings.Contains(name, m.openPath)) {
		return nil, m.openError
	}
	return m.base.Open(name)
}

// mockErrorFile wraps a file and injects read errors
type mockErrorFile struct {
	base      fs.File
	readError error
}

func (m *mockErrorFile) Read(b []byte) (int, error) {
	if m.readError != nil {
		return 0, m.readError
	}
	return m.base.Read(b)
}

func (m *mockErrorFile) Close() error {
	return m.base.Close()
}

func (m *mockErrorFile) Stat() (fs.FileInfo, error) {
	return m.base.Stat()
}

// mockErrorDirFs creates a filesystem that always fails on specific directory operations
type mockErrorDirFs struct {
	base fs.FS
}

func (m *mockErrorDirFs) Open(name string) (fs.File, error) {
	f, err := m.base.Open(name)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// Inject error for directory reads
	if info.IsDir() && name == "unreadable" {
		f.Close()
		return nil, fs.ErrPermission
	}

	return f, nil
}

// mockLstatFs wraps a filesystem with Lstat support that can inject errors
type mockLstatFs struct {
	base       fs.FS
	lstatError error
	lstatPath  string
}

func (m *mockLstatFs) Open(name string) (fs.File, error) {
	return m.base.Open(name)
}

func (m *mockLstatFs) Lstat(name string) (fs.FileInfo, error) {
	if m.lstatError != nil && (m.lstatPath == "" || strings.Contains(name, m.lstatPath)) {
		return nil, m.lstatError
	}
	// Fallback to regular stat if the underlying FS doesn't support Lstat
	return fs.Stat(m.base, name)
}
