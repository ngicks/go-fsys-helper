package implwrapper

import (
	"io/fs"
	"time"

	"github.com/spf13/afero"
)

//go:generate go run ../../cmd/implwrapper -pkg ./ -fsys A1,A2 -file B1,B2

// no methods
type A1 struct {
	inner afero.Fs
}

type A2 struct {
	inner afero.Fs
}

func (fsys *A2) modifyPath(_ string, oldname string, newname string) (string, string) {
	return oldname, newname
}

func (fsys *A2) modifyMode(_ string, perm fs.FileMode) fs.FileMode {
	return perm
}

func (fsys *A2) modifyFile(_ string, file afero.File) afero.File {
	return &B2{inner: file}
}

func (fsys *A2) modifyErr(_ string, err error) error {
	return err
}

func (fsys *A2) modifyFi(_ string, fi []fs.FileInfo) []fs.FileInfo {
	return fi
}

func (fsys *A2) beforeEach(_ string, _ ...any) error {
	return nil
}

func (fsys *A2) afterEach(_ string, _ ...any) error {
	return nil
}

func (fsys *A2) modifyTimes(_ string, atime, mtime time.Time) (time.Time, time.Time) {
	return atime, mtime
}

type B1 struct {
	inner afero.File
}

type B2 struct {
	inner afero.File
}

func (b *B2) modifyFi(_ string, fi []fs.FileInfo) []fs.FileInfo {
	return fi
}

func (b *B2) modifyDirnames(_ string, s []string) []string {
	return s
}

func (b *B2) modifyP(_ string, p []byte) []byte {
	return p
}

func (b *B2) modifyOff(_ string, off int64) int64 {
	return off
}

func (b *B2) modifyN(_ string, n int) int {
	return n
}

func (b *B2) modifyString(_ string, s string) string {
	return s
}

func (b *B2) modifyErr(_ string, err error) error {
	return err
}

func (b *B2) beforeEach(_ string, _ ...any) error {
	return nil
}

func (b *B2) afterEach(_ string, _ ...any) error {
	return nil
}
