package fsutil

import (
	"os"
	"slices"
	"sync"
	"time"

	"github.com/spf13/afero"
)

var _ afero.Fs = (*ObservableFs)(nil)

type ObservableFsOpName string

const (
	ObservableFsOpNameCreate    = "Create"
	ObservableFsOpNameMkdir     = "Mkdir"
	ObservableFsOpNameMkdirAll  = "MkdirAll"
	ObservableFsOpNameOpen      = "Open"
	ObservableFsOpNameOpenFile  = "OpenFile"
	ObservableFsOpNameRemove    = "Remove"
	ObservableFsOpNameRemoveAll = "RemoveAll"
	ObservableFsOpNameRename    = "Rename"
	ObservableFsOpNameStat      = "Stat"
	ObservableFsOpNameChmod     = "Chmod"
	ObservableFsOpNameChown     = "Chown"
	ObservableFsOpNameChtimes   = "Chtimes"
)

type ObservableFsFileOpName string

const (
	ObservableFsFileOpNameClose        = "Close"
	ObservableFsFileOpNameRead         = "Read"
	ObservableFsFileOpNameReadAt       = "ReadAt"
	ObservableFsFileOpNameSeek         = "Seek"
	ObservableFsFileOpNameWrite        = "Write"
	ObservableFsFileOpNameWriteAt      = "WriteAt"
	ObservableFsFileOpNameName         = "Name"
	ObservableFsFileOpNameReaddir      = "Readdir"
	ObservableFsFileOpNameReaddirnames = "Readdirnames"
	ObservableFsFileOpNameStat         = "Stat"
	ObservableFsFileOpNameSync         = "Sync"
	ObservableFsFileOpNameTruncate     = "Truncate"
	ObservableFsFileOpNameWriteString  = "WriteString"
)

type ObservableFsOp struct {
	Name string
	Op   ObservableFsOpName
	Args []any
	Err  error
}

type ObservableFsFileOp struct {
	Name string
	Op   ObservableFsFileOpName
	Args []any
	Err  error
}

type Observer struct {
	o *ObservableFs
}

func (o *Observer) FsOp() []ObservableFsOp {
	return o.o.readFsOp()
}

func (o *Observer) FileOp(name string) []ObservableFsFileOp {
	return o.o.readFileOp(name)
}

func (o *Observer) FileOps() map[string][]ObservableFsFileOp {
	return o.o.readFileOps()
}

type ObservableFs struct {
	mu     sync.Mutex
	base   afero.Fs
	fsysOp []ObservableFsOp
	fileOp map[string][]ObservableFsFileOp
}

func NewObservableFs(base afero.Fs) *ObservableFs {
	return &ObservableFs{
		base:   base,
		fsysOp: make([]ObservableFsOp, 0),
		fileOp: make(map[string][]ObservableFsFileOp),
	}
}

func (fsys *ObservableFs) readFsOp() []ObservableFsOp {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	return slices.Clone(fsys.fsysOp)
}

func (fsys *ObservableFs) readFileOp(name string) []ObservableFsFileOp {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	name = normalizePath(name)
	return slices.Clone(fsys.fileOp[name])
}

func (fsys *ObservableFs) readFileOps() map[string][]ObservableFsFileOp {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	out := make(map[string][]ObservableFsFileOp)
	for k, v := range fsys.fileOp {
		out[k] = slices.Clone(v)
	}
	return out
}

func (fsys *ObservableFs) recordFsOp(name string, op ObservableFsOpName, args []any, err error) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	fsys.fsysOp = append(fsys.fsysOp, ObservableFsOp{normalizePath(name), op, args, err})
}

func (fsys *ObservableFs) recordFileOp(name string, op ObservableFsFileOpName, args []any, err error) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	name = normalizePath(name)
	fsys.fileOp[name] = append(fsys.fileOp[name], ObservableFsFileOp{name, op, args, err})
}

func (fsys *ObservableFs) Observer() *Observer {
	return &Observer{fsys}
}

func (fsys *ObservableFs) Create(name string) (afero.File, error) {
	f, err := fsys.base.Create(name)
	fsys.recordFsOp(name, ObservableFsOpNameCreate, nil, err)
	return newObservableFile(fsys, normalizePath(name), f, err)
}
func (fsys *ObservableFs) Mkdir(name string, perm os.FileMode) error {
	err := fsys.base.Mkdir(name, perm)
	fsys.recordFsOp(name, ObservableFsOpNameMkdir, []any{perm}, err)
	return err
}
func (fsys *ObservableFs) MkdirAll(path string, perm os.FileMode) error {
	err := fsys.base.MkdirAll(path, perm)
	fsys.recordFsOp(path, ObservableFsOpNameMkdirAll, []any{perm}, err)
	return err
}
func (fsys *ObservableFs) Open(name string) (afero.File, error) {
	f, err := fsys.base.Open(name)
	fsys.recordFsOp(name, ObservableFsOpNameOpen, nil, err)
	return newObservableFile(fsys, normalizePath(name), f, err)
}
func (fsys *ObservableFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := fsys.base.OpenFile(name, flag, perm)
	fsys.recordFsOp(name, ObservableFsOpNameOpenFile, []any{flag, perm}, err)
	return newObservableFile(fsys, normalizePath(name), f, err)
}
func (fsys *ObservableFs) Remove(name string) error {
	err := fsys.base.Remove(name)
	fsys.recordFsOp(name, ObservableFsOpNameRemove, nil, err)
	return err
}
func (fsys *ObservableFs) RemoveAll(path string) error {
	err := fsys.base.RemoveAll(path)
	fsys.recordFsOp(path, ObservableFsOpNameRemoveAll, nil, err)
	return err
}
func (fsys *ObservableFs) Rename(oldname, newname string) error {
	err := fsys.base.Rename(oldname, newname)
	fsys.recordFsOp(oldname, ObservableFsOpNameRename, []any{newname}, err)
	return err
}
func (fsys *ObservableFs) Stat(name string) (os.FileInfo, error) {
	stat, err := fsys.base.Stat(name)
	fsys.recordFsOp(name, ObservableFsOpNameStat, nil, err)
	return stat, err
}
func (fsys *ObservableFs) Name() string {
	return fsys.base.Name()
}
func (fsys *ObservableFs) Chmod(name string, mode os.FileMode) error {
	err := fsys.base.Chmod(name, mode)
	fsys.recordFsOp(name, ObservableFsOpNameChmod, []any{mode}, err)
	return err
}
func (fsys *ObservableFs) Chown(name string, uid, gid int) error {
	err := fsys.base.Chown(name, uid, gid)
	fsys.recordFsOp(name, ObservableFsOpNameChown, []any{uid, gid}, err)
	return err
}
func (fsys *ObservableFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	err := fsys.base.Chtimes(name, atime, mtime)
	fsys.recordFsOp(name, ObservableFsOpNameChtimes, []any{atime, mtime}, err)
	return err
}

var _ afero.File = (*observableFile)(nil)

type observableFile struct {
	observer *ObservableFs
	path     string
	f        afero.File
}

func newObservableFile(observer *ObservableFs, path string, f afero.File, err error) (afero.File, error) {
	if err != nil {
		return nil, err
	}
	return &observableFile{
		path:     path,
		observer: observer,
		f:        f,
	}, nil
}

func (f *observableFile) record(op ObservableFsFileOpName, args []any, err error) {
	f.observer.recordFileOp(f.path, op, args, err)
}

func (f *observableFile) Close() error {
	err := f.f.Close()
	f.record(ObservableFsFileOpNameClose, nil, err)
	return err
}
func (f *observableFile) Read(p []byte) (n int, err error) {
	n, err = f.f.Read(p)
	f.record(ObservableFsFileOpNameRead, nil, err)
	return n, err
}
func (f *observableFile) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = f.f.ReadAt(p, off)
	f.record(ObservableFsFileOpNameReadAt, []any{off}, err)
	return n, err
}
func (f *observableFile) Seek(offset int64, whence int) (int64, error) {
	n, err := f.f.Seek(offset, whence)
	f.record(ObservableFsFileOpNameSeek, []any{offset, whence}, err)
	return n, err
}
func (f *observableFile) Write(p []byte) (n int, err error) {
	n, err = f.f.Write(p)
	f.record(ObservableFsFileOpNameWrite, nil, err)
	return n, err
}
func (f *observableFile) WriteAt(p []byte, off int64) (n int, err error) {
	n, err = f.f.WriteAt(p, off)
	f.record(ObservableFsFileOpNameWriteAt, []any{off}, err)
	return n, err
}
func (f *observableFile) Name() string {
	return f.f.Name()
}
func (f *observableFile) Readdir(count int) ([]os.FileInfo, error) {
	dirent, err := f.f.Readdir(count)
	f.record(ObservableFsFileOpNameReaddir, []any{count}, err)
	return dirent, err
}
func (f *observableFile) Readdirnames(n int) ([]string, error) {
	names, err := f.f.Readdirnames(n)
	f.record(ObservableFsFileOpNameReaddirnames, []any{n}, err)
	return names, err
}
func (f *observableFile) Stat() (os.FileInfo, error) {
	s, err := f.f.Stat()
	f.record(ObservableFsFileOpNameStat, nil, err)
	return s, err
}
func (f *observableFile) Sync() error {
	err := f.f.Sync()
	f.record(ObservableFsFileOpNameSync, nil, err)
	return err
}
func (f *observableFile) Truncate(size int64) error {
	err := f.f.Truncate(size)
	f.record(ObservableFsFileOpNameTruncate, []any{size}, err)
	return err
}
func (f *observableFile) WriteString(s string) (ret int, err error) {
	ret, err = f.f.WriteString(s)
	f.record(ObservableFsFileOpNameWriteString, []any{s}, err)
	return ret, err
}
