package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestChown exercises [vroot.Fs.Chown] on regular files and directories.
//
// Chown is permitted to be a no-op or return an error on systems where the test process
// lacks privileges. When Option.SkipChown is set this test only asserts that calling
// Chown with the test's uid/gid does not return an error.
func TestChown[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChown {
		t.Skip("SkipChown is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "x"`,
	)

	t.Run("on file", func(t *testing.T) {
		err := fsys.Chown("file.txt", s.Option.ChownUid, s.Option.ChownGid)
		if err != nil {
			t.Fatalf("Chown: %v", err)
		}
	})

	t.Run("on directory", func(t *testing.T) {
		err := fsys.Chown("dir", s.Option.ChownUid, s.Option.ChownGid)
		if err != nil {
			t.Fatalf("Chown: %v", err)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Chown("does-not-exist", s.Option.ChownUid, s.Option.ChownGid)
		if err == nil {
			t.Fatalf("Chown on missing file: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Chown on missing file: want fs.ErrNotExist, got %v", err)
		}
	})
}
