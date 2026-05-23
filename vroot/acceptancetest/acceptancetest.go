// Package acceptancetest defines acceptance tests for [vroot.Fs] and [vroot.Root] implementations.
//
// Callers select OS-specific assertions via [Option.Os] instead of build tags so that
// implementations targeting either family of behavior can be exercised from a single binary.
package acceptancetest

import (
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Os selects the family of OS-specific behaviors a test should expect.
type Os int

const (
	// OsEnv is the zero value: OS-specific behavior is auto-detected from
	// [runtime.GOOS] at test time. POSIX-like targets resolve to [OsUnix],
	// windows to [OsWindows]; plan9 and other non-POSIX targets are rejected
	// (the test fails) so an unsupported platform never silently runs the
	// wrong assertions. Set [OsUnix] or [OsWindows] explicitly to override.
	OsEnv Os = iota
	// OsUnix means unix-like behavior: chmod respects bits, symlinks freely allowed, etc.
	OsUnix
	// OsWindows means Windows behavior: chmod only flips the read-only bit, symlinks may require privileges, etc.
	OsWindows
)

func (o Os) String() string {
	switch o {
	case OsEnv:
		return "env"
	case OsUnix:
		return "unix"
	case OsWindows:
		return "windows"
	}
	return "unknown"
}

// resolve returns the concrete OS family. When o is [OsEnv] it consults
// [runtime.GOOS]: windows -> [OsWindows], POSIX-like -> [OsUnix], anything
// else fails the test (plan9, js, wasip1, …) since the acceptance assertions
// only model the unix and windows families.
func (o Os) resolve(t *testing.T) Os {
	t.Helper()
	if o != OsEnv {
		return o
	}
	switch runtime.GOOS {
	case "windows":
		return OsWindows
	case "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
		return OsUnix
	default:
		t.Fatalf("acceptancetest: GOOS %q is neither POSIX-like nor windows; set Option.Os explicitly", runtime.GOOS)
		return OsEnv
	}
}

// Option describes capabilities and expected behavior of the implementation under test.
//
// The zero value enables every test. Set Skip* flags to opt out of tests that depend
// on a capability the implementation does not provide.
type Option struct {
	// Os selects which OS-specific assertions are used.
	Os Os

	// SkipSeek skips tests of [vroot.File.Seek]. Implementations that return
	// [vroot.ErrOpNotSupported] from Seek should set this.
	SkipSeek bool
	// SkipReadAt skips tests of [vroot.File.ReadAt].
	SkipReadAt bool
	// SkipWriteAt skips tests of [vroot.File.WriteAt].
	SkipWriteAt bool
	// SkipSymlink skips tests of Symlink and ReadLink, and skips symlink-dependent cases
	// in other tests (Stat-follow, Lstat-of-link, Remove-symlink, escapes-via-symlink).
	SkipSymlink bool
	// SkipHardlink skips tests of Link.
	SkipHardlink bool
	// SkipChmod skips tests of Chmod and [vroot.File.Chmod].
	SkipChmod bool
	// SkipChown skips tests of Chown, Lchown, and [vroot.File.Chown].
	SkipChown bool
	// ChownUid and ChownGid are the (uid, gid) passed to Chown/Lchown/file.Chown.
	// They are only consulted when SkipChown is false. Zero values are typically fine.
	ChownUid int
	ChownGid int
	// SkipChtimes skips tests of Chtimes.
	SkipChtimes bool
	// SkipRename skips tests of Rename.
	SkipRename bool
	// SkipRemoveAllDotComponent skips the RemoveAll sub-test that asserts a
	// path whose final element is "." (e.g. "." or "tree/.") fails with EINVAL.
	//
	// Thin path-normalizing wrappers (separator converters, prefix wrappers)
	// run filepath.Clean before delegating, collapsing "tree/." to "tree", so
	// they cannot observe the trailing dot. Such wrappers should set this;
	// real filesystem implementations must leave it false.
	SkipRemoveAllDotComponent bool
}

// Setup describes how to build a fresh [vroot.Fs] for a test.
//
// Make is invoked once per test or sub-test and must register any cleanup it
// requires on t. The returned Fs must be rooted at a fresh directory
// pre-populated with the entries described by lines (using the same syntax as
// [github.com/ngicks/go-fsys-helper/fsutil/testhelper.ParseSetupProcLine]).
// lines may be empty.
//
// Read-write Fs implementations typically materialize lines through the Fs's
// own write methods. Read-only Fs implementations must materialize lines
// out-of-band before constructing the read-only view and should be exercised
// via [RunFsReadOnly] rather than [RunFs].
type Setup[F vroot.File, Fs vroot.Fs[F]] struct {
	Make func(t *testing.T, lines []string) Fs

	Option Option
}

// SetupRoot is the [vroot.Root]-typed counterpart of [Setup].
type SetupRoot[F vroot.File, R vroot.Root[F, R]] struct {
	Make func(t *testing.T, lines []string) R

	Option Option
}
