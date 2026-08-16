//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package osfs_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// probeFlock reports the error of taking how (LOCK_SH / LOCK_EX) on f right now.
// LOCK_NB makes the probe answer instead of blocking, which the blocking
// [vroot.Locker] API cannot do; nil means the lock was granted (and taken).
func probeFlock(t *testing.T, f *osfs.File, how int) error {
	t.Helper()
	return syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
}

func openLockTarget(t *testing.T, fsys *osfs.Fs) *osfs.File {
	t.Helper()
	f, err := fsys.OpenFile("f.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TestFileLock checks that osfs files lock through [vroot.Locker] against each
// other. flock is per open file description, so the two handles opened here
// contend even though they live in the same process.
func TestFileLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fsys, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	t.Cleanup(func() { _ = fsys.Close() })

	h1 := openLockTarget(t, fsys)
	h2 := openLockTarget(t, fsys)

	l1, ok := any(h1).(vroot.Locker)
	if !ok {
		t.Fatalf("%T does not implement vroot.Locker", h1)
	}
	l2, ok := any(h2).(vroot.Locker)
	if !ok {
		t.Fatalf("%T does not implement vroot.Locker", h2)
	}

	// Two shared holders coexist.
	if err := l1.Lock(vroot.LockShared); err != nil {
		t.Fatalf("h1.Lock(LockShared): %v", err)
	}
	if err := l2.Lock(vroot.LockShared); err != nil {
		t.Fatalf("h2.Lock(LockShared): %v", err)
	}

	// Upgrading h2 while h1 still holds shared has to wait, so the non-blocking
	// probe reports EWOULDBLOCK instead. Linux drops h2's shared lock when a
	// conversion fails, so nothing below assumes h2 still holds it.
	if err := probeFlock(t, h2, syscall.LOCK_EX); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("LOCK_EX|LOCK_NB on h2 while h1 holds shared = %v, want EWOULDBLOCK", err)
	}

	if err := l1.Unlock(); err != nil {
		t.Fatalf("h1.Unlock: %v", err)
	}

	// With h1 released, the exclusive lock is available.
	if err := probeFlock(t, h2, syscall.LOCK_EX); err != nil {
		t.Fatalf("LOCK_EX|LOCK_NB on h2 after h1 unlocked = %v, want nil", err)
	}

	// Exclusive excludes even shared holders.
	if err := probeFlock(t, h1, syscall.LOCK_SH); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("LOCK_SH|LOCK_NB on h1 while h2 holds exclusive = %v, want EWOULDBLOCK", err)
	}

	if err := l2.Unlock(); err != nil {
		t.Fatalf("h2.Unlock: %v", err)
	}
	if err := probeFlock(t, h1, syscall.LOCK_SH); err != nil {
		t.Fatalf("LOCK_SH|LOCK_NB on h1 after h2 unlocked = %v, want nil", err)
	}
}

// TestFileLockInvalidLevel checks that a level outside the two documented
// constants is rejected instead of being passed to flock.
func TestFileLockInvalidLevel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fsys, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	t.Cleanup(func() { _ = fsys.Close() })

	f := openLockTarget(t, fsys)
	if err := f.Lock(vroot.LockLevel(0)); !errors.Is(err, syscall.EINVAL) {
		t.Fatalf("Lock(LockLevel(0)) = %v, want EINVAL", err)
	}
}
