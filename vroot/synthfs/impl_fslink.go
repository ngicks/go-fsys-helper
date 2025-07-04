package synthfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

func readonlyFsysErr(op, name string) error {
	return &fs.PathError{Op: op, Path: name, Err: errdef.EROFS}
}

var _ FileView = (*fsFileView)(nil)

type fsFileView struct {
	fsys fs.FS
	path string
}

func (b *fsFileView) Close() error {
	return nil
}

// NewFsFileView builds FileData that points a file stored in fsys referred as path.
func NewFsFileView(fsys fs.FS, path string) (FileView, error) {
	return newFsFileView(fsys, path)
}

func newFsFileView(fsys fs.FS, path string) (*fsFileView, error) {
	s, err := fs.Stat(fsys, path)
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
		return nil, &fs.PathError{Op: "NewFsLinkFileData", Path: path, Err: syscall.EISDIR}
	}
	if !s.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "NewFsLinkFileData", Path: path, Err: errdef.EBADF}
	}
	return &fsFileView{fsys, path}, nil
}

func (b *fsFileView) Open(flag int) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	f, err := b.fsys.Open(b.path)
	if err != nil {
		return nil, err
	}
	return vroot.ExpandFsFile(f, b.path), nil
}

func (b *fsFileView) Stat() (fs.FileInfo, error) {
	return fs.Stat(b.fsys, b.path)
}

var _ FileView = (*vrootFsFileView)(nil)

type vrootFsFileView struct {
	fsys vroot.Fs
	path string
}

func (v *vrootFsFileView) Close() error {
	return nil
}

// NewVrootFsFileView builds FileView that points a file stored in vroot.Fs referred as path.
func NewVrootFsFileView(fsys vroot.Fs, path string) (FileView, error) {
	return newVrootFsFileView(fsys, path)
}

func newVrootFsFileView(fsys vroot.Fs, path string) (*vrootFsFileView, error) {
	s, err := fsys.Stat(path)
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
		return nil, &fs.PathError{Op: "NewVrootFsFileView", Path: path, Err: syscall.EISDIR}
	}
	if !s.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "NewVrootFsFileView", Path: path, Err: errdef.EBADF}
	}
	return &vrootFsFileView{fsys, path}, nil
}

func (v *vrootFsFileView) Open(flag int) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	return v.fsys.Open(v.path)
}

func NewRangedFsFileView(fsys fs.FS, path string, off, n int64) (FileView, error) {
	fd, err := newFsFileView(fsys, path)
	if err != nil {
		return nil, err
	}
	return NewRangedFileView(fd, off, n)
}

func NewRangedFileView(fd FileView, off, n int64) (FileView, error) {
	if off < 0 {
		return nil, fmt.Errorf("off must not be negative = %d", off)
	}
	if n <= 0 {
		return nil, fmt.Errorf("n must be greater than 0")
	}

	f, err := fd.Open(os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r, ok := f.(io.ReaderAt)
	if !ok {
		return nil, fmt.Errorf("fsys must open io.ReaderAt implementor")
	}

	// check implementation
	var b [1]byte
	_, err = r.ReadAt(b[:], 0)
	if err != nil {
		return nil, fmt.Errorf("fsys must open io.ReaderAt implementor: %w", err)
	}

	return &rangedFileView{off, n, fd}, nil
}

type rangedFileView struct {
	off, n int64
	FileView
}

func (b *rangedFileView) Close() error {
	return b.FileView.Close()
}

func (b *rangedFileView) Open(flag int) (vroot.File, error) {
	f, err := b.FileView.Open(flag)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		return nil, err
	}
	sr := io.NewSectionReader(f, b.off, b.n)
	return &sectionFile{s.Name(), f, sr}, nil
}

func (b *rangedFileView) Stat() (fs.FileInfo, error) {
	s, err := statFileView(b.FileView)
	if err != nil {
		return nil, err
	}
	return stat{
		mode:    s.Mode(),
		modTime: s.ModTime(),
		name:    s.Name(),
		size:    int64(b.n) - int64(b.off),
	}, nil
}

var _ vroot.File = (*sectionFile)(nil)

type sectionFile struct {
	path string
	f    fs.File
	*io.SectionReader
}

func (s *sectionFile) permErr(op string) error {
	return fsutil.WrapPathErr(op, s.path, fs.ErrPermission)
}

func (s *sectionFile) Chmod(mode fs.FileMode) error {
	return s.permErr("chmod")
}

func (s *sectionFile) Chown(uid int, gid int) error {
	return s.permErr("chown")
}

func (s *sectionFile) Fd() uintptr {
	return ^(uintptr(0))
}

func (s *sectionFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return []fs.DirEntry{}, fsutil.WrapPathErr("readdir", s.path, syscall.ENOTDIR)
}

func (s *sectionFile) Close() error {
	return s.f.Close()
}

// Name implements afero.File.
func (s *sectionFile) Name() string {
	return s.path
}

// Readdir implements afero.File.
func (s *sectionFile) Readdir(count int) ([]fs.FileInfo, error) {
	return []fs.FileInfo{}, fsutil.WrapPathErr("readdir", s.path, syscall.ENOTDIR)
}

// Readdirnames implements afero.File.
func (s *sectionFile) Readdirnames(n int) ([]string, error) {
	return []string{}, fsutil.WrapPathErr("readdir", s.path, syscall.ENOTDIR)
}

// Stat implements afero.File.
func (s *sectionFile) Stat() (fs.FileInfo, error) {
	st, err := s.f.Stat()
	if err != nil {
		return nil, err
	}
	return stat{
		mode:    st.Mode(),
		modTime: st.ModTime(),
		name:    path.Base(s.path),
		size:    s.SectionReader.Size(),
	}, nil
}

// Sync implements afero.File.
func (s *sectionFile) Sync() error {
	// file is readonly
	return nil
}

// Truncate implements afero.File.
func (s *sectionFile) Truncate(size int64) error {
	return readonlyFsysErr("truncate", s.path)
}

// Write implements afero.File.
func (s *sectionFile) Write(p []byte) (n int, err error) {
	return 0, readonlyFsysErr("write", s.path)
}

// WriteAt implements afero.File.
func (s *sectionFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, readonlyFsysErr("writeat", s.path)
}

// WriteString implements afero.File.
func (s *sectionFile) WriteString(_ string) (ret int, err error) {
	return 0, readonlyFsysErr("write", s.path)
}
