// Package acceptancetest defines acceptance tests for [vroot.Fs] and [vroot.Root] implementations.
//
// Callers select OS-specific assertions via [Option.Os] instead of build tags so that
// implementations targeting either family of behavior can be exercised from a single binary.
package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Os selects the family of OS-specific behaviors a test should expect.
type Os int

const (
	// OsUnix means unix-like behavior: chmod respects bits, symlinks freely allowed, etc.
	OsUnix Os = iota
	// OsWindows means Windows behavior: chmod only flips the read-only bit, symlinks may require privileges, etc.
	OsWindows
)

func (o Os) String() string {
	switch o {
	case OsUnix:
		return "unix"
	case OsWindows:
		return "windows"
	}
	return "unknown"
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
}

// Setup describes how to build a fresh [vroot.Fs] for a test.
//
// The constructor is invoked once per test or sub-test and must register any cleanup
// it requires on t. The returned Fs must be rooted at a fresh, empty directory.
type Setup[F vroot.File, Fs vroot.Fs[F]] struct {
	// Make builds a fresh, empty file system. Implementations should register
	// cleanup via t.Cleanup so the test framework can release resources.
	Make func(t *testing.T) Fs

	Option Option
}

// SetupRoot is the [vroot.Root]-typed counterpart of [Setup].
type SetupRoot[F vroot.File, R vroot.Root[F, R]] struct {
	Make func(t *testing.T) R

	Option Option
}
