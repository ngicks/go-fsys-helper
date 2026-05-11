package acceptancetest

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestSymlink exercises [vroot.Fs.Symlink].
//
// Symlink stores the target string verbatim; the target need not exist at creation time.
// The newly created link is observable via Lstat with the ModeSymlink bit set.
func TestSymlink[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipSymlink {
		t.Skip("SkipSymlink is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`target.txt: "x"`)

	t.Run("link to existing target", func(t *testing.T) {
		c.Symlink("target.txt", "lnk")

		info, err := fsys.Lstat("lnk")
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("symlink mode missing: got mode=%s", info.Mode())
		}

		got, err := fsys.ReadLink("lnk")
		if err != nil {
			t.Fatalf("ReadLink: %v", err)
		}
		want := filepath.FromSlash("target.txt")
		if got != want {
			t.Errorf("ReadLink: got %q, want %q", got, want)
		}
	})

	t.Run("link to non-existent target is allowed", func(t *testing.T) {
		c.Symlink("nothing-here", "broken")

		info, err := fsys.Lstat("broken")
		if err != nil {
			t.Fatalf("Lstat broken: %v", err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("broken link should still be a symlink, got mode=%s", info.Mode())
		}
	})

	t.Run("target already exists", func(t *testing.T) {
		c.SetupLines(`occupied.txt: "x"`)
		err := fsys.Symlink("anywhere", "occupied.txt")
		if err == nil {
			t.Fatalf("Symlink with occupied newname: want error, got nil")
		}
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("Symlink with occupied newname: want fs.ErrExist, got %v", err)
		}
	})

	t.Run("parent of new path does not exist", func(t *testing.T) {
		err := fsys.Symlink("target.txt", "missing-parent/link")
		if err == nil {
			t.Fatalf("Symlink with missing parent: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Symlink with missing parent: want fs.ErrNotExist, got %v", err)
		}
	})
}
