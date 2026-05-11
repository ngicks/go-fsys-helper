package acceptancetest

import (
	"fmt"
	"strings"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
)

// RootFsys is a fixture tree used by walk and similar tests.
//
// The tree contains both "readable" content meant for read-only assertions and
// "writable" content meant for mutation tests. Outside the root there are paths
// that escapes deliberately point at.
var RootFsys = []string{
	"outside/",
	"outside/dir/",
	"root/",
	"root/readable/",
	"root/readable/subdir/",
	"root/readable/subdir/double_nested/",
	"root/writable/",
	"root/writable/subdir/",
	`outside/outside_file.txt: "foofoofoo"`,
	`outside/dir/nested_outside.txt: "barbarbar"`,
	`root/readable/file1.txt: "bazbazbaz"`,
	`root/readable/file2.txt: "quxquxqux"`,
	`root/readable/subdir/nested_file.txt: "nested_file"`,
	"root/readable/subdir/symlink_upward -> ../symlink_inner",
	"root/readable/subdir/symlink_upward_escapes -> ../symlink_escapes",
	`root/readable/subdir/double_nested/double_nested.txt: "double_nested"`,
	"root/readable/symlink_escapes -> ../../outside/outside_file.txt",
	"root/readable/symlink_escapes_dir -> ../../outside/dir",
	"root/readable/symlink_inner -> ./file1.txt",
	"root/readable/symlink_inner_dir -> ./subdir",
	`root/writable/file1.txt: "baz"`,
	`root/writable/file2.txt: "qux"`,
	`root/writable/subdir/nested_file.txt: "nested_file"`,
}

// MakeOsFsys populates tempDir on the host file system with [RootFsys] entries.
//
// Set readable=true to include the read-only tree under root/readable/.
// Set writable=true to include the writable tree under root/writable/.
func MakeOsFsys(tempDir string, readable, writable bool) {
	for _, line := range RootFsys {
		if !readable && strings.HasPrefix(line, "root/readable") {
			continue
		}
		if !writable && strings.HasPrefix(line, "root/writable") {
			continue
		}
		if err := testhelper.ExecuteLineOs(tempDir, line); err != nil {
			panic(fmt.Errorf("MakeOsFsys: %w", err))
		}
	}
}
