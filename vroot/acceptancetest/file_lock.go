package acceptancetest

import (
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileLock exercises the optional [vroot.Locker] extension.
//
// Locker is opt-in per implementation rather than an [Option] flag: the test
// type-asserts the interface on an opened file and skips when the file does not
// implement it.
func TestFileLock[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `f.txt: "x"`)
	c := newC(t, fsys)

	f := c.OpenFile("f.txt", os.O_RDWR, 0)
	defer func() { _ = f.Close() }()

	l, ok := any(f).(vroot.Locker)
	if !ok {
		t.Skipf("%T does not implement vroot.Locker", f)
	}

	testhelper.NilErr(t, l.Lock(vroot.LockShared))
	// Locking again converts the held lock instead of failing or deadlocking.
	testhelper.NilErr(t, l.Lock(vroot.LockExclusive))
	testhelper.NilErr(t, l.Unlock())

	// The file is lockable again once released.
	testhelper.NilErr(t, l.Lock(vroot.LockExclusive))
	testhelper.NilErr(t, l.Unlock())
}
