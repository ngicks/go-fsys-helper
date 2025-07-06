package synthfs

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

func must1(err error) {
	if err != nil {
		panic(err)
	}
}

func prep(fsys vroot.Fs) {
	// Create all content under root/, not just writable
	must1(
		acceptancetest.ExecuteAllLineDirection(
			fsys,
			acceptancetest.FilterLineDirection(
				func(l acceptancetest.LineDirection) bool { return strings.HasPrefix(l.Path, "root/") },
				slices.Values(acceptancetest.RootFsysDirections),
			),
		),
	)
}

func TestRooted(t *testing.T) {
	opt := Option{
		Clock:                  clock.RealWallClock(),
		DisableOpenFileRemoval: runtime.GOOS == "windows",
		MaskChmodMode:          fsutil.MaskChmodMode,
	}
	r := NewRooted("synth", NewMemFileAllocator(clock.RealWallClock()), opt)
	prep(r)
	rr, err := r.OpenRoot(filepath.FromSlash("root/writable"))
	must1(err)
	defer rr.Close()
	acceptancetest.RootedReadWrite(t, rr)
}

func TestUnrooted(t *testing.T) {
	opt := Option{
		Clock:                  clock.RealWallClock(),
		DisableOpenFileRemoval: runtime.GOOS == "windows",
		MaskChmodMode:          fsutil.MaskChmodMode,
	}
	r := NewUnrooted("synth", NewMemFileAllocator(clock.RealWallClock()), opt)
	prep(r)
	rr, err := r.OpenUnrooted(filepath.FromSlash("root/writable"))
	must1(err)
	acceptancetest.UnrootedReadWrite(t, rr, false)
}

func TestUmaskZero(t *testing.T) {
	zeroUmask := fs.FileMode(0)
	opt := Option{
		Clock: clock.RealWallClock(),
		Umask: &zeroUmask,
	}
	r := NewRooted("synth", NewMemFileAllocator(clock.RealWallClock()), opt)

	// Create a file with 0777 permissions
	err := vroot.WriteFile(r, "test.txt", []byte("test"), 0o777)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Check that the file has the full permissions (not masked)
	info, err := r.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// With zero umask, the file should have 0777 permissions
	if info.Mode().Perm() != 0o777 {
		t.Errorf("Expected file permissions 0777, got %o", info.Mode().Perm())
	}

	// Test with directory
	err = r.Mkdir("testdir", 0o755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	dirInfo, err := r.Stat("testdir")
	if err != nil {
		t.Fatalf("Stat testdir failed: %v", err)
	}

	// With zero umask, the directory should have 0755 permissions
	if dirInfo.Mode().Perm() != 0o755 {
		t.Errorf("Expected directory permissions 0755, got %o", dirInfo.Mode().Perm())
	}
}
