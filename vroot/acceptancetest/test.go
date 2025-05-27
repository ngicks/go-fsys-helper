package acceptancetest

import (
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// anything, keep this to let linking work.
var _ = (*os.File)(nil)

var RootFsys = []string{
	"outside/",
	"outside/dir/",
	"root/",
	"root/readable/",
	"root/readable/subdir/",
	"root/readable/subdir/double_nested/",
	"root/writable/",
	"root/writable/subdir/",
	"outside/outside_file.txt: foofoofoo",
	"outside/dir/nested_outside.txt: barbarbar",
	"root/readable/file1.txt: bazbazbaz",
	"root/readable/file2.txt: quxquxqux",
	"root/readable/subdir/nested_file.txt: nested_file",
	"root/readable/subdir/symlink_upward -> ../symlink_inner",
	"root/readable/subdir/symlink_upward_escapes -> ../symlink_escapes",
	"root/readable/subdir/double_nested/double_nested.txt: double_nested",
	"root/readable/symlink_escapes -> ../../outside/outside_file.txt",
	"root/readable/symlink_escapes_dir -> ../../outside/dir",
	"root/readable/symlink_inner -> ./file1.txt",
	"root/readable/symlink_inner_dir -> ./subdir",
	"root/writable/file1.txt: baz",
	"root/writable/file2.txt: qux",
	"root/writable/subdir/nested_file.txt: nested_file",
}

// RootedReadOnly tests implemention of [vroot.Rooted] assuming it is read-only implementation.
// If the implementation is read/write then call [RootedReadWrite] instead.
//
// RootedReadOnly assmues given rooted is read-only. Tests will call writing method,
// e.g. Chmod, OpenFile with [os.O_RDWR], Create, must fails with soem error.
//
// RootedReadOnly places assumption of rooted file system as follows:
//
//   - outside/
//   - outside/outside_file.txt: foofoofoo
//   - outside/dir/
//   - outside/dir/nested_outside.txt: barbarbar
//   - root/
//   - root/readable/
//   - root/readable/file1.txt: bazbazbaz
//   - root/readable/file2.txt: quxquxqux
//   - root/readable/subdir/
//   - root/readable/subdir/nested_file.txt: nested_file
//   - root/readable/subdir/symlink_upward -> ../symlink_inner
//   - root/readable/subdir/symlink_upward_escapes -> ../symlink_escapes
//   - root/readable/subdir/double_nested/
//   - root/readable/subdir/double_nested/double_nested.txt: double_nested
//   - root/readable/symlink_escapes -> ../../outside/outside_file.txt
//   - root/readable/symlink_escapes_dir -> ../../outside/dir
//   - root/readable/symlink_inner -> ./file1.txt
//   - root/readable/symlink_inner_dir -> ./subdir
//
// The input [vroot.Rooted] must be rooted at root/readable/.
//
// Names which end with
//   - /  are directies.
//   - :  are file. Its content follows after a white space.
//   - -> are symlinks. Its target follows after a white space.
//
// You can prepare up fsys by using [RootFsys].
//
// Permissions are not strictly considered since some system widens or narrows them.
// rooted must be rooted at root/
//
// [vroot.Rooted] must prohibit path traversal to upward or symlin escape from root.
// When escapes fail methods must return [vroot.ErrPathEscapes].
func RootedReadOnly(t *testing.T, rooted vroot.Rooted) {
	t.Run("read file", func(t *testing.T) {
		readFile(t, rooted)
	})
	t.Run("read directory", func(t *testing.T) {
		readDirectory(t, rooted)
	})
	t.Run("write fails", func(t *testing.T) {
		writeFails(t, rooted)
	})
	t.Run("follow symlink", func(t *testing.T) {
		followSymlink(t, rooted)
	})
	t.Run("follow symlink fails for escapes", func(t *testing.T) {
		followSymlinkFailsForEscapes(t, rooted)
	})
	t.Run("path traversal fails", func(t *testing.T) {
		pathTraversalFails(t, rooted)
	})
	t.Run("sub root", func(t *testing.T) {
		subRootReadOnly(t, rooted)
	})
}

// RootedReadWrite tests implemention of [vroot.Rooted] assuming it is read/write implementation.
// If the implementation is read-only then call [RootedReadOnly] instead.
//
// RootedReadWrite places assumption of rooted file system as follows:
//
//   - outside/
//   - outside/outside_file.txt: foofoo
//   - outside/dir/
//   - outside/dir/nested_outside.txt: barbar
//   - root/
//   - root/writable/
//   - root/writable/file1.txt: baz
//   - root/writable/file2.txt: qux
//   - root/writable/subdir/
//   - root/writable/subdir/nested_file.txt: nested_file
//
// The input [vroot.Rooted] must be rooted at root/writable/.
//
// Names which end with
//   - /  are directies.
//   - :  are file. Its content follows after a white space.
//
// The test will try to write to both under outside/ root/writable/.
// The test will writes content and eventually under root/writable/ populated like under root/readable described [RootedReadOnly].
//
// Permissions are not strictly considered since some system widens or narrows them.
// rooted must be rooted at root/
//
// Some tests try to create symlinks to ./outside under ./root/.
// [vroot.Rooted] must prohibit path traversal to upward or symlin escape from root.
// When escapes fail methods must return [vroot.ErrPathEscapes].
func RootedReadWrite(t *testing.T, rooted vroot.Rooted) {
	t.Run("write", func(t *testing.T) {
		write(t, rooted)
	})
	// populate rest of content.
	populateRoot(t, rooted)
	t.Run("read file", func(t *testing.T) {
		readFile(t, rooted)
	})
	t.Run("read directory", func(t *testing.T) {
		readDirectory(t, rooted)
	})
	t.Run("follow symlink", func(t *testing.T) {
		followSymlink(t, rooted)
	})
	t.Run("follow symlink fails for escapes", func(t *testing.T) {
		followSymlinkFailsForEscapes(t, rooted)
	})
	t.Run("path traversal fails", func(t *testing.T) {
		pathTraversalFails(t, rooted)
	})
	t.Run("sub root", func(t *testing.T) {
		subRootReadWrite(t, rooted)
	})
}

// UnrootedReadOnly is same as [RootedReadOnly] but allows symlink escape.
// If implementation can not have outside/ dir then pass false value to hasOutside.
func UnrootedReadOnly(t *testing.T, unrooted vroot.Unrooted, hasOutside bool) {
	// Do basically same as RootedReadOnly but allow symlink escape.
	t.Run("read file", func(t *testing.T) {
		readFile(t, unrooted)
	})
	t.Run("read directory", func(t *testing.T) {
		readDirectory(t, unrooted)
	})
	t.Run("write fails", func(t *testing.T) {
		writeFails(t, unrooted)
	})
	t.Run("follow symlink", func(t *testing.T) {
		followSymlink(t, unrooted)
	})
	t.Run("follow symlink fails for escapes", func(t *testing.T) {
		followSymlinkAllowedForEscapes(t, unrooted, hasOutside)
	})
	t.Run("path traversal fails", func(t *testing.T) {
		pathTraversalFails(t, unrooted)
	})
	t.Run("sub root", func(t *testing.T) {
		subRootReadOnly(t, unrooted)
	})
	t.Run("sub unroot", func(t *testing.T) {
		subUnrootedReadOnly(t, unrooted)
	})
}

// UnrootedReadOnly is same as [RootedReadWRite] but accepts [vroot.Unrooted].
// If implementation can not have outside/ dir then pass false value to hasOutside.
func UnrootedReadWrite(t *testing.T, unrooted vroot.Unrooted, hasOutside bool) {
	// Do basically same as RootedReadOnly but allow symlink escape.
	t.Run("write", func(t *testing.T) {
		write(t, unrooted)
	})
	// populate rest of content.
	populateRoot(t, unrooted)
	t.Run("read file", func(t *testing.T) {
		readFile(t, unrooted)
	})
	t.Run("read directory", func(t *testing.T) {
		readDirectory(t, unrooted)
	})
	t.Run("follow symlink", func(t *testing.T) {
		followSymlink(t, unrooted)
	})
	t.Run("follow symlink allowed for escapes", func(t *testing.T) {
		followSymlinkAllowedForEscapes(t, unrooted, hasOutside)
	})
	t.Run("path traversal fails", func(t *testing.T) {
		pathTraversalFails(t, unrooted)
	})
	t.Run("sub root", func(t *testing.T) {
		subRootReadWrite(t, unrooted)
	})
	t.Run("sub unroot", func(t *testing.T) {
		subUnrootedReadWrite(t, unrooted)
	})
}
