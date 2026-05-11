package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// newC builds a testhelper.C around an Fs. The Test type parameter is *testing.T.
func newC[F vroot.File, Fs vroot.Fs[F]](t *testing.T, fsys Fs) *testhelper.C[*testing.T, F, Fs] {
	return testhelper.New(t, fsys)
}

// makeFs creates a new Fs from the Setup and registers Close() via t.Cleanup.
func makeFs[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) Fs {
	t.Helper()
	fsys := s.Make(t)
	t.Cleanup(func() {
		_ = fsys.Close()
	})
	return fsys
}

// makeRoot creates a new Root from the SetupRoot and registers Close() via t.Cleanup.
func makeRoot[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) R {
	t.Helper()
	r := s.Make(t)
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r
}
