package testhelper

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

var _ Test[*testing.T] = (*testing.T)(nil)

type Test[T Test[T]] interface {
	ArtifactDir() string
	Attr(key string, value string)
	Chdir(dir string)
	Cleanup(f func())
	Context() context.Context
	Deadline() (deadline time.Time, ok bool)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Helper()
	Log(args ...any)
	Logf(format string, args ...any)
	Name() string
	Output() io.Writer
	Parallel()
	Run(name string, f func(t T)) bool
	Setenv(key string, value string)
	Skip(args ...any)
	SkipNow()
	Skipf(format string, args ...any)
	Skipped() bool
	TempDir() string
}

type Fsys[F File] interface {
	Chmod(name string, mode fs.FileMode) error
	Chown(name string, uid int, gid int) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Create(name string) (F, error)
	Lchown(name string, uid int, gid int) error
	Link(oldname string, newname string) error
	Lstat(name string) (fs.FileInfo, error)
	Mkdir(name string, perm fs.FileMode) error
	MkdirAll(name string, perm fs.FileMode) error
	Open(name string) (F, error)
	OpenFile(name string, flag int, perm fs.FileMode) (F, error)
	ReadLink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname string, newname string) error
	Stat(name string) (fs.FileInfo, error)
	Symlink(oldname string, newname string) error
}

type File interface {
	Chmod(mode fs.FileMode) error
	Chown(uid int, gid int) error
	Close() error

	// Fd returns internal detail of file handle.
	// Only os-backed File should reutrn this value.
	// Otherwise, return ^(uintptr(0)) to indicate this is invalid value.
	Fd() uintptr

	Name() string
	Read(b []byte) (n int, err error)
	ReadAt(b []byte, off int64) (n int, err error)
	ReadDir(n int) ([]fs.DirEntry, error)

	Readdir(n int) ([]fs.FileInfo, error)
	Readdirnames(n int) (names []string, err error)
	Seek(offset int64, whence int) (ret int64, err error)

	Stat() (fs.FileInfo, error)
	Sync() error

	Truncate(size int64) error
	Write(b []byte) (n int, err error)
	WriteAt(b []byte, off int64) (n int, err error)
	WriteString(s string) (n int, err error)
}

// C represents the curent test context.
// It assumulates up operaions happened in a test case
// and reports detailedly when something went wrong.
type C[T Test[T], F File, Fs Fsys[F]] struct {
	t       T
	fsys    Fs
	history []string
}

func New[T Test[T], F File, Fs Fsys[F]](t T, fsys Fs) *C[T, F, Fs] {
	return &C[T, F, Fs]{t: t, fsys: fsys}
}

func (c *C[T, F, Fs]) record(format string, args ...any) {
	c.history = append(c.history, fmt.Sprintf(format, args...))
}

func (c *C[T, F, Fs]) ReportFailf(format string, args ...any) {
	c.t.Helper()
	msg := fmt.Sprintf(format, args...)
	if len(c.history) == 0 {
		c.t.Fatalf("%s", msg)
	}

	var b strings.Builder
	b.WriteString(msg)
	b.WriteString("\noperation history:")
	for _, h := range c.history {
		b.WriteString("\n  - ")
		b.WriteString(h)
	}
	c.t.Fatalf("%s", b.String())
}
