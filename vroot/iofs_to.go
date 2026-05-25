package vroot

import (
	"errors"
	"io/fs"
)

// Compile-time interface checks. Written as generic helpers so we don't have
// to pick a concrete F/R pair from inside vroot.
func _[F File]() {
	var _ fs.FS = (*ioFs[F])(nil)
	var _ fs.ReadDirFS = (*ioFs[F])(nil)
	var _ fs.ReadFileFS = (*ioFs[F])(nil)
	var _ fs.ReadLinkFS = (*ioFs[F])(nil)
	var _ fs.StatFS = (*ioFs[F])(nil)
	var _ fs.SubFS = (*ioFs[F])(nil)
}

func _[F File, R Root[F, R]]() {
	var _ fs.FS = (*ioFsRoot[F, R])(nil)
	var _ fs.ReadDirFS = (*ioFsRoot[F, R])(nil)
	var _ fs.ReadFileFS = (*ioFsRoot[F, R])(nil)
	var _ fs.ReadLinkFS = (*ioFsRoot[F, R])(nil)
	var _ fs.StatFS = (*ioFsRoot[F, R])(nil)
	var _ fs.SubFS = (*ioFsRoot[F, R])(nil)
}

// ioFs adapts an [Fs] to [fs.FS]. The returned [fs.FS] satisfies
// [fs.ReadDirFS], [fs.ReadFileFS], [fs.ReadLinkFS], [fs.StatFS] and
// [fs.SubFS]. Sub is implemented through [Sub] (a native SubFs when the inner
// supports it, otherwise a [PathPrefixFs] view).
//
// ioFs does not translate path separators. Paths arriving from [fs.FS]
// callers are forward-slash form per [fs.ValidPath]; they are forwarded to
// the inner Fs as-is. If the inner Fs expects platform-specific paths (e.g.
// [osfs.Fs]), wrap it with [ToOsPathFs] first so callers can keep using
// forward slashes.
type ioFs[F File] struct {
	inner Fs[F]
}

// ToIoFs wraps fsys as an [fs.FS]. The returned filesystem additionally
// implements [fs.ReadDirFS], [fs.ReadFileFS], [fs.ReadLinkFS], [fs.StatFS] and
// [fs.SubFS]; callers can type-assert as needed.
func ToIoFs[F File](fsys Fs[F]) fs.FS {
	return &ioFs[F]{inner: fsys}
}

func (i *ioFs[F]) Close() error {
	return i.inner.Close()
}

func (i *ioFs[F]) Open(name string) (fs.File, error) {
	if err := validFsPath("open", name); err != nil {
		return nil, err
	}
	return narrowFile(i.inner.Open(name))
}

func (i *ioFs[F]) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := validFsPath("readdir", name); err != nil {
		return nil, err
	}
	return ReadDir(i.inner, name)
}

func (i *ioFs[F]) ReadFile(name string) ([]byte, error) {
	if err := validFsPath("readfile", name); err != nil {
		return nil, err
	}
	return ReadFile(i.inner, name)
}

func (i *ioFs[F]) ReadLink(name string) (string, error) {
	if err := validFsPath("readlink", name); err != nil {
		return "", err
	}
	return i.inner.ReadLink(name)
}

func (i *ioFs[F]) Lstat(name string) (fs.FileInfo, error) {
	if err := validFsPath("lstat", name); err != nil {
		return nil, err
	}
	return i.inner.Lstat(name)
}

func (i *ioFs[F]) Stat(name string) (fs.FileInfo, error) {
	if err := validFsPath("stat", name); err != nil {
		return nil, err
	}
	return i.inner.Stat(name)
}

// Sub implements [fs.SubFS] via [Sub]: a native SubFs on the inner Fs if it
// has one, otherwise a [PathPrefixFs] view rooted at dir.
func (i *ioFs[F]) Sub(dir string) (fs.FS, error) {
	if err := validFsPath("sub", dir); err != nil {
		return nil, err
	}
	sub, err := Sub(i.inner, dir)
	if err != nil {
		return nil, err
	}
	return ToIoFs(sub), nil
}

// ioFsRoot adapts a [Root] to [fs.FS]. In addition to what [ioFs] supports,
// the returned filesystem also satisfies [fs.SubFS]: Sub uses OpenRoot on the
// inner [Root] so the sub-filesystem stays rooted.
type ioFsRoot[F File, R Root[F, R]] struct {
	inner R
}

// ToIoFsRoot wraps r as an [fs.FS] with [fs.SubFS] support. The returned
// filesystem also implements [fs.ReadDirFS], [fs.ReadFileFS], [fs.ReadLinkFS]
// and [fs.StatFS]; callers can type-assert as needed.
func ToIoFsRoot[F File, R Root[F, R]](r R) fs.FS {
	return &ioFsRoot[F, R]{inner: r}
}

func (i *ioFsRoot[F, R]) Close() error {
	return i.inner.Close()
}

func (i *ioFsRoot[F, R]) Open(name string) (fs.File, error) {
	if err := validFsPath("open", name); err != nil {
		return nil, err
	}
	return narrowFile(i.inner.Open(name))
}

func (i *ioFsRoot[F, R]) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := validFsPath("readdir", name); err != nil {
		return nil, err
	}
	return ReadDir(i.inner, name)
}

func (i *ioFsRoot[F, R]) ReadFile(name string) ([]byte, error) {
	if err := validFsPath("readfile", name); err != nil {
		return nil, err
	}
	return ReadFile(i.inner, name)
}

func (i *ioFsRoot[F, R]) ReadLink(name string) (string, error) {
	if err := validFsPath("readlink", name); err != nil {
		return "", err
	}
	return i.inner.ReadLink(name)
}

func (i *ioFsRoot[F, R]) Lstat(name string) (fs.FileInfo, error) {
	if err := validFsPath("lstat", name); err != nil {
		return nil, err
	}
	return i.inner.Lstat(name)
}

func (i *ioFsRoot[F, R]) Stat(name string) (fs.FileInfo, error) {
	if err := validFsPath("stat", name); err != nil {
		return nil, err
	}
	return i.inner.Stat(name)
}

func (i *ioFsRoot[F, R]) Sub(dir string) (fs.FS, error) {
	if err := validFsPath("sub", dir); err != nil {
		return nil, err
	}
	sub, err := i.inner.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return ToIoFsRoot(sub), nil
}

// validFsPath enforces the [fs.ValidPath] contract at the [fs.FS] boundary.
// The wrapped vroot.Fs / vroot.Root may accept looser paths (e.g. *os.Root
// tolerates "a//b"), but an fs.FS must reject anything that is not a clean,
// unrooted, slash-separated path.
func validFsPath(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

// fsFile narrows a [File] to [fs.File] capability.
type fsFile struct {
	f File
}

// fsFileReaderAt is [fsFile] additionally exposing [io.ReaderAt] and
// [io.Seeker]. It is selected by [NarrowFile] when the underlying file
// successfully serves a probe ReadAt call.
type fsFileReaderAt struct {
	*fsFile
}

// NarrowFile returns an [fs.File] backed by f. If a probe ReadAt on f reports
// [ErrOpNotSupported], the returned [fs.File] does not implement [io.ReaderAt]
// nor [io.Seeker]; otherwise it does. Other ReadAt errors (e.g. io.EOF on an
// empty file) are ignored so that ReaderAt-capable files keep their full
// capability surface.
func NarrowFile(f File) fs.File {
	var b [1]byte
	_, readAtErr := f.ReadAt(b[:], 0)
	if !errors.Is(readAtErr, ErrOpNotSupported) {
		return &fsFileReaderAt{&fsFile{f: f}}
	}
	return &fsFile{f: f}
}

func narrowFile(f File, err error) (fs.File, error) {
	// Check err first: an Fs whose File type is a pointer (e.g. *os.File)
	// returns a typed-nil on failure, which is a non-nil File interface here.
	// Testing f == nil would therefore wrap the nil and hide the error.
	if err != nil {
		return nil, err
	}
	return NarrowFile(f), nil
}

func (r *fsFile) Close() error {
	return r.f.Close()
}

func (r *fsFile) Name() string {
	return r.f.Name()
}

func (r *fsFile) Read(b []byte) (n int, err error) {
	return r.f.Read(b)
}

func (r *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return r.f.ReadDir(n)
}

func (r *fsFile) Readdir(n int) ([]fs.FileInfo, error) {
	return r.f.Readdir(n)
}

func (r *fsFile) Readdirnames(n int) (names []string, err error) {
	return r.f.Readdirnames(n)
}

func (r *fsFile) Stat() (fs.FileInfo, error) {
	return r.f.Stat()
}

func (r *fsFileReaderAt) Seek(offset int64, whence int) (ret int64, err error) {
	return r.f.Seek(offset, whence)
}

func (r *fsFileReaderAt) ReadAt(b []byte, off int64) (n int, err error) {
	return r.f.ReadAt(b, off)
}
