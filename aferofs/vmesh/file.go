package vmesh

import (
	"errors"
	"io/fs"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

var _ afero.File = (*virtualFile)(nil)

type virtualFile struct {
	meta *virtualFileData
	afero.File
}

func (v *virtualFile) Name() string {
	return v.meta.name
}

func (v *virtualFile) Stat() (fs.FileInfo, error) {
	return v.meta.StatFile(v.File)
}

type virtualFileData struct {
	file FileData

	mu          sync.RWMutex
	initialized bool
	name        string
	mode        fs.FileMode
	uid, gid    int
	modTime     time.Time
}

func newVirtualFileData(f FileData, name string) (*virtualFileData, error) {
	s, err := f.Stat()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &virtualFileData{
				file: f,
				name: name,
			}, nil
		}
		return nil, err
	}
	if s.IsDir() {
		return nil, syscall.EISDIR
	}
	vfd := &virtualFileData{
		file: f,
		name: name,
	}
	err = vfd.init(s, nil)
	if err != nil {
		return nil, err
	}
	return vfd, nil
}

func (v *virtualFileData) init(s fs.FileInfo, f afero.File) error {
	v.mu.RLock()
	if v.initialized {
		v.mu.RUnlock()
		return nil
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		// check agin: race.
		return nil
	}

	if s == nil && f != nil {
		var err error
		s, err = f.Stat()
		if err != nil {
			return err
		}
	}

	v.mode = s.Mode()
	v.modTime = s.ModTime()

	v.initialized = true

	return nil
}

func (v *virtualFileData) Close() error {
	return v.file.Close()
}

func (v *virtualFileData) Open(flag int) (afero.File, error) {
	f, err := v.file.Open(flag)
	if err != nil {
		return nil, err
	}
	err = v.init(nil, f)
	if err != nil {
		return nil, err
	}
	return &virtualFile{meta: v, File: f}, nil
}

func (v *virtualFileData) Stat() (fs.FileInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	s, err := v.file.Stat()
	if err != nil {
		return nil, err
	}
	return stat{v.mode, v.modTime, v.name, s.Size()}, nil
}

func (v *virtualFileData) StatFile(f afero.File) (fs.FileInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	s, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return stat{v.mode, v.modTime, v.name, s.Size()}, nil
}

func (v *virtualFileData) Truncate(size int64) error {
	return v.file.Truncate(size)
}

func (v *virtualFileData) Chmod(mode fs.FileMode) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.mode = mode & (fs.ModeType | fs.ModePerm)
}

func (v *virtualFileData) Chown(uid, gid int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.uid, v.gid = uid, gid
}

func (v *virtualFileData) Chtimes(_, mtime time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !mtime.IsZero() {
		v.modTime = mtime
	}
}

func (v *virtualFileData) Mode() fs.FileMode {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.mode
}

func (v *virtualFileData) Owner() (uid, gid int) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.uid, v.gid
}

func (v *virtualFileData) Times() (atime, mtime time.Time) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return time.Time{}, v.modTime
}
