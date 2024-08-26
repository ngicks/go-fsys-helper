package vmesh

import (
	"crypto/rand"
	"embed"
	_ "embed"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ngicks/go-fsys-helper/aferofs"
	"github.com/ngicks/go-fsys-helper/aferofs/clock"
	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

//go:embed testdata
var randomBytes embed.FS

func TestFstest(t *testing.T) {
	clock := clock.RealWallClock()
	fsys := New(0, NewMemFileAllocator(clock))

	var err error
	err = fsys.MkdirAll("foo", fs.ModePerm)
	assert.NilError(t, err)
	err = fsys.MkdirAll("foo/bar", fs.ModePerm)
	assert.NilError(t, err)

	paths := []string{
		"ah", "oh", "foo_random0",
		"foo/foofoo1", "foo/foofoo2", "foo/foo_random1",
		"foo/bar/baz0", "foo/bar/baz1", "foo/bar/baz3", "foo/bar/foo_random2",
	}

	for _, name := range paths {
		f, err := fsys.OpenFile(name, os.O_CREATE|os.O_RDWR, fs.ModePerm)
		assert.NilError(t, err)
		_, err = io.CopyN(f, rand.Reader, 64*1024)
		assert.NilError(t, err)
	}

	for _, s := range [][2]string{
		{"random0", "foo_random0"},
		{"random1", "foo/foo_random1"},
		{"random2", "foo/bar/foo_random2"},
	} {
		d, err := NewFsFileView(randomBytes, "testdata/"+s[0])
		assert.NilError(t, err)
		err = fsys.AddFile(s[1], d)
		assert.NilError(t, err)
		err = fsys.Chmod(s[1], fs.ModePerm)
		assert.NilError(t, err)
	}

	err = fstest.TestFS(
		&aferofs.IoFs{Fs: fsys},
		paths...,
	)
	assert.NilError(t, err)
}

func TestMkdir(t *testing.T) {
	clock := clock.RealWallClock()

	current := clock.Now()

	time.Sleep(time.Millisecond)

	var (
		fsys *Fs
		err  error
	)

	fsys = New(0, NewMemFileAllocator(clock))

	err = fsys.Mkdir("foo/bar", 0)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	err = fsys.Mkdir("foo", fs.ModePerm)
	assert.NilError(t, err)
	err = fsys.Mkdir("foo/barbar", 0)
	assert.NilError(t, err)
	err = fsys.Mkdir("foo/bar/baz", 0)
	assert.ErrorIs(t, err, fs.ErrNotExist)

	err = fsys.MkdirAll("foo/bar/baz", fs.ModePerm)
	assert.NilError(t, err)
	assertMkdirAll(t, fsys, "foo/bar/baz", fs.ModePerm, current, true)

	fsys = New(0o022, NewMemFileAllocator(clock))
	assert.NilError(t, fsys.MkdirAll("foo/bar/baz", fs.ModePerm))
	assertMkdirAll(t, fsys, "foo/bar/baz", 0o755, current, true)
	err = fsys.Mkdir("foo/barbar", fs.ModePerm)
	assert.NilError(t, err)
	assertMkdir(t, fsys, "foo/barbar", 0o755, current, true)

	fsys = New(0o022, NewMemFileAllocator(clock))
	assert.NilError(t, fsys.MkdirAll("foo/bar/baz", 0o711))
	assertMkdirAll(t, fsys, "foo/bar/baz", 0o711, current, true)
	err = fsys.Mkdir("foo/barbar", 0o733)
	assert.NilError(t, err)
	assertMkdir(t, fsys, "foo/barbar", 0o711, current, true)

	_, err = fsys.Create("foo/ah")
	assert.NilError(t, err)
	err = fsys.Mkdir("foo/ah", 0)
	assert.ErrorIs(t, err, fs.ErrExist)
	err = fsys.MkdirAll("foo/ah/oh", 0)
	t.Log(err)
	assert.ErrorIs(t, err, syscall.ENOTDIR)
}

func assertMkdir(
	t *testing.T,
	fsys afero.Fs,
	path string, perm fs.FileMode, current time.Time, nilSys bool,
) {
	s, err := fsys.Stat(path)
	assert.NilError(t, err)
	assertDirStat(t, s, pathpkg.Base(path), perm, current, nilSys)
}

func assertMkdirAll(
	t *testing.T,
	fsys afero.Fs,
	path string, perm fs.FileMode, current time.Time, nilSys bool,
) {
	for {
		s, err := fsys.Stat(path)
		assert.NilError(t, err)
		assertDirStat(t, s, pathpkg.Base(path), perm, current, nilSys)
		idx := strings.LastIndex(path, "/")
		if idx < 0 {
			break
		}
		path = path[:idx]
	}
}

func assertDirStat(
	t *testing.T,
	s fs.FileInfo,
	name string, perm fs.FileMode, current time.Time, nilSys bool,
) {
	assertStat(t, s, name, 4096, perm, current, true, nilSys)
}

func assertStat(
	t *testing.T,
	s fs.FileInfo,
	name string, size int64, perm fs.FileMode, current time.Time, isDir bool, nilSys bool,
) {
	assert.Equal(t, s.Name(), name)
	assert.Equal(t, s.Size(), size)
	assert.Equal(t, s.Mode().Perm(), perm)
	assert.Assert(t, s.ModTime().Compare(current) > 0)
	assert.Equal(t, s.IsDir(), isDir)
	assert.Assert(t, (s.Sys() == nil) == nilSys)
}
