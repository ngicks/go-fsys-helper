package acceptancetest

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"syscall"
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

	// Moving a directory into its own subtree must fail with EINVAL, matching
	// POSIX rename(2). A naive re-parent would detach the parent chain into a
	// cycle, so this also guards against ".."-walk hangs. Pairs with plan 01 V2.
	t.Run("directory into own subtree", func(t *testing.T) {
		c.SetupLines(
			"mvdir/",
			"mvdir/inner/",
			`mvdir/inner/keep.txt: "k"`,
		)
		// Direct child and deeper descendant are both rejected.
		err := fsys.Rename("mvdir", filepath.Join("mvdir", "inner", "moved"))
		testhelper.ErrIs(t, err, syscall.EINVAL)
		err = fsys.Rename("mvdir", filepath.Join("mvdir", "child"))
		testhelper.ErrIs(t, err, syscall.EINVAL)
		// The tree must remain intact and walkable after the rejection.
		_, err = fsys.Stat(filepath.Join("mvdir", "inner", "keep.txt"))
		testhelper.NilErr(t, err)
	})

	// NOTE: "rename a path onto itself" is intentionally NOT asserted here.
	// It is not a portable contract: Linux rename(2) of a *directory* onto
	// itself returns EEXIST through plain os.Rename (the osfs.Fs path), while
	// *os.Root.Rename and synthfs treat it as a no-op. synthfs's own no-op
	// guarantee is covered by synthfs/rename_test.go (plan 01 V2).
}
