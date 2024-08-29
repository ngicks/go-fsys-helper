package aferofs

import (
	"slices"
	"sync"
	"time"

	"github.com/spf13/afero"
)

type ObservableFsHistory struct {
	Timestamp time.Time
	IsFsys    bool
	IsBefore  bool
	Method    string
	Args      []any
}

type ObservableFs struct {
	mu      sync.Mutex
	history []ObservableFsHistory
	inner   afero.Fs
}

func (fsys *ObservableFs) modifyFile(_ string, file afero.File) afero.File {
	return &ObservableFile{inner: file, fsys: fsys}
}

func (fsys *ObservableFs) beforeEach(method string, args ...any) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	fsys.history = append(fsys.history, ObservableFsHistory{
		Timestamp: time.Now(),
		IsFsys:    true,
		IsBefore:  true,
		Method:    method,
		Args:      slices.Clone(args),
	})
}

func (fsys *ObservableFs) afterEach(method string, args ...any) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	fsys.history = append(fsys.history, ObservableFsHistory{
		Timestamp: time.Now(),
		IsFsys:    true,
		IsBefore:  false,
		Method:    method,
		Args:      slices.Clone(args),
	})
}

type ObservableFile struct {
	inner afero.File
	fsys  *ObservableFs
}

func (f *ObservableFile) beforeEach(method string, args ...any) {
	f.fsys.mu.Lock()
	defer f.fsys.mu.Unlock()
	f.fsys.history = append(f.fsys.history, ObservableFsHistory{
		Timestamp: time.Now(),
		IsFsys:    false,
		IsBefore:  true,
		Method:    method,
		Args:      slices.Clone(args),
	})
}

func (f *ObservableFile) afterEach(method string, args ...any) {
	f.fsys.mu.Lock()
	defer f.fsys.mu.Unlock()
	f.fsys.history = append(f.fsys.history, ObservableFsHistory{
		Timestamp: time.Now(),
		IsFsys:    false,
		IsBefore:  false,
		Method:    method,
		Args:      slices.Clone(args),
	})
}
