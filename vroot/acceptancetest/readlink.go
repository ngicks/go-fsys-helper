package acceptancetest

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestReadLink exercises [vroot.Fs.ReadLink].
func TestReadLink[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipSymlink {
		t.Skip("SkipSymlink is set")
	}

	fsys := makeFs(t, s,
		`target.txt: "x"`,
		"link -> target.txt",
		"deep -> some/nested/place",
	)

	t.Run("returns target verbatim", func(t *testing.T) {
		got, err := fsys.ReadLink("link")
		testhelper.NilErr(t, err)
		// The target was written through SetupLines using filepath.FromSlash.
		want := filepath.FromSlash("target.txt")
		if got != want {
			t.Errorf("ReadLink(link): got %q, want %q", got, want)
		}
	})

	t.Run("nested path target", func(t *testing.T) {
		got, err := fsys.ReadLink("deep")
		testhelper.NilErr(t, err)
		want := filepath.FromSlash("some/nested/place")
		if got != want {
			t.Errorf("ReadLink(deep): got %q, want %q", got, want)
		}
	})

	t.Run("not a symlink", func(t *testing.T) {
		_, err := fsys.ReadLink("target.txt")
		if err == nil {
			t.Fatalf("ReadLink on regular file: want error, got nil")
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		_, err := fsys.ReadLink("does-not-exist")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
