package acceptancetest

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileWriteAt exercises [vroot.File.WriteAt].
//
// When Option.SkipWriteAt is set, the implementation may return [vroot.ErrOpNotSupported].
func TestFileWriteAt[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "ABCDEF"`)

	f := c.OpenFile("f.txt", os.O_RDWR, 0)
	defer func() { _ = f.Close() }()

	if s.Option.SkipWriteAt {
		_, err := f.WriteAt([]byte("z"), 0)
		if err != nil && !errors.Is(err, vroot.ErrOpNotSupported) {
			t.Errorf("WriteAt on unsupported file: want ErrOpNotSupported or success, got %v", err)
		}
		return
	}

	_, err := f.WriteAt([]byte("xyz"), 2)
	testhelper.NilErr(t, err)
	_ = f.Close()

	r := c.Open("f.txt")
	defer func() { _ = r.Close() }()
	got, err := io.ReadAll(r)
	testhelper.NilErr(t, err)
	if string(got) != "ABxyzF" {
		t.Errorf("after WriteAt: got %q, want %q", got, "ABxyzF")
	}
}
