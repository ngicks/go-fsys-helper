package aferofs

import (
	"slices"
	"sync"
	"time"

	"github.com/spf13/afero"
)

//go:generate go run ./cmd/implwrapper -pkg ./ -fsys ObservableFs -file ObservableFile

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

func (fsys *ObservableFs) recordOp(isFsys bool, IsBefore bool, method string, args []any) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	fsys.history = append(fsys.history, ObservableFsHistory{
		Timestamp: time.Now(),
		IsFsys:    isFsys,
		IsBefore:  IsBefore,
		Method:    method,
		Args:      slices.Clone(args),
	})
}

func (fsys *ObservableFs) modifyFile(_ string, file afero.File) afero.File {
	if file == nil {
		return nil
	}
	return &ObservableFile{inner: file, fsys: fsys}
}

func (fsys *ObservableFs) beforeEach(method string, args ...any) error {
	fsys.recordOp(true, true, method, args)
	return nil
}

func (fsys *ObservableFs) afterEach(method string, args ...any) error {
	fsys.recordOp(true, false, method, args)
	return nil
}

type ObservableFile struct {
	inner afero.File
	fsys  *ObservableFs
}

func (f *ObservableFile) beforeEach(method string, args ...any) error {
	f.fsys.recordOp(false, true, method, args)
	return nil
}

func (f *ObservableFile) afterEach(method string, args ...any) error {
	f.fsys.recordOp(false, false, method, args)
	return nil
}
