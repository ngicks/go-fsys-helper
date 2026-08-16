package acceptancetest

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRename exercises [vroot.Fs.Rename] with assertions that hold on every
// supported platform. Platform-specific assertions (e.g. POSIX overwrite
// semantics) live in sibling tests and are dispatched by [RunFsReadWrite].
func TestRename[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipRename {
		t.Skip("SkipRename is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("file", func(t *testing.T) {
		c.SetupLines(`old.txt: "x"`)
		c.Rename("old.txt", "new.txt")
		_, err := fsys.Stat("old.txt")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
		_, err = fsys.Stat("new.txt")
		testhelper.NilErr(t, err)
	})

	t.Run("directory", func(t *testing.T) {
		c.SetupLines(
			"olddir/",
			`olddir/inside.txt: "x"`,
		)
		c.Rename("olddir", "newdir")
		_, err := fsys.Stat("olddir")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
		_, err = fsys.Stat("newdir/inside.txt")
		testhelper.NilErr(t, err)
	})

	t.Run("source does not exist", func(t *testing.T) {
		err := fsys.Rename("missing", "anything")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}

// TestRenameUnix exercises POSIX rename(2)-specific behavior. It is dispatched
// by [RunFsReadWrite] only when [Option.Os] is [OsUnix]; Windows' MoveFile
// semantics are stricter and do not honor these assertions.
func TestRenameUnix[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipRename {
		t.Skip("SkipRename is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("overwrites existing file", func(t *testing.T) {
		c.SetupLines(
			`src.txt: "fresh"`,
			`dst.txt: "stale"`,
		)
		c.Rename("src.txt", "dst.txt")
		_, err := fsys.Stat("src.txt")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
		got, err := vroot.ReadFile(fsys, "dst.txt")
		testhelper.NilErr(t, err)
		if !bytes.Equal(got, []byte("fresh")) {
			t.Errorf("dst content after overwrite: got %q, want %q", got, "fresh")
		}
	})
}
