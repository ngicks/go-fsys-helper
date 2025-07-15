package testhelper

import (
	"bytes"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"testing"
)

func OpenFile[
	Fsys interface {
		OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	},
	File interface {
		io.Closer
	},
](
	t T,
	fsys Fsys,
	path string,
	flag int,
	perm fs.FileMode,
	fn func(t ExtendedT, file File),
) {
	tt := wrapT(t).PushOp("OpenFile").PushPath(path)
	tt.Helper()
	f, err := fsys.OpenFile(filepath.FromSlash(path), flag, perm.Perm())
	if err != nil {
		tt.Errorf("failed: %v", err)
		return
	}
	defer f.Close()
	fn(tt, f)
}

func Open[
	Fsys interface {
		Open(name string) (File, error)
	},
	File interface {
		io.Closer
	},
](
	t *testing.T,
	fsys Fsys,
	path string,
	fn func(t ExtendedT, file File),
) {
	tt := wrapT(t).PushOp("Open").PushPath(path)
	tt.Helper()
	f, err := fsys.Open(filepath.FromSlash(path))
	if err != nil {
		tt.Errorf("failed: %v", err)
		return
	}
	defer f.Close()
	fn(tt, f)
}

func Create[
	Fsys interface {
		Create(name string) (File, error)
	},
	File interface {
		io.Closer
	},
](
	t *testing.T,
	fsys Fsys,
	path string,
	fn func(t ExtendedT, file File),
) {
	tt := wrapT(t).PushOp("Create").PushPath(path)
	tt.Helper()
	f, err := fsys.Create(filepath.FromSlash(path))
	if err != nil {
		tt.Errorf("failed: %v", err)
		return
	}
	defer f.Close()
	fn(tt, f)
}

func AssertContent[
	Fsys interface {
		Open(name string) (File, error)
	},
	File interface {
		io.Reader
		io.Closer
	},
](t ExtendedT, fsys Fsys, path string, content []byte) {
	tt := t.PushOp("Open").PushPath(path)
	tt.Helper()

	f, err := fsys.Open(path)
	if err != nil {
		tt.Errorf("failed: %v", err)
		return
	}
	defer f.Close()

	tt = t.PushOp("io.ReadAll").PushPath(path)
	bin, err := io.ReadAll(f)
	if err != nil {
		tt = tt.PushContext("read bytes", len(bin))
		tt.Errorf("failed: %v", err)
		return
	}

	if bytes.Equal(content, bin) {
		return
	}

	t.
		PushPath(path).
		PushContext("expected", strconv.Quote(string(content))).
		PushContext("actual", strconv.Quote(string(bin))).
		Error("not equal")
}
