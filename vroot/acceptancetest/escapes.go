package acceptancetest

import (
	"errors"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// test symlink resolusion fails if it escapes the root.
// call Open or similar methods and check if it fails with [vroot.ErrPathEscapes]
func followSymlinkFailsForEscapes(t *testing.T, rooted vroot.Rooted) {
	escapingSymlinks := []string{
		"symlink_escapes",               // -> ../../outside/outside_file.txt
		"symlink_escapes_dir",           // -> ../../outside/dir
		"subdir/symlink_upward_escapes", // -> ../symlink_escapes
	}

	for _, linkName := range escapingSymlinks {
		t.Run(linkName, func(t *testing.T) {
			// Test Open should fail with ErrPathEscapes
			_, err := rooted.Open(linkName)
			if err == nil {
				t.Errorf("Open %q should have failed with path escape error", linkName)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Open %q failed with %v, expected ErrPathEscapes", linkName, err)
			}

			// Test Stat should fail with ErrPathEscapes
			_, err = rooted.Stat(linkName)
			if err == nil {
				t.Errorf("Stat %q should have failed with path escape error", linkName)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Stat %q failed with %v, expected ErrPathEscapes", linkName, err)
			}

			// Test OpenFile should fail with ErrPathEscapes
			_, err = rooted.OpenFile(linkName, 0, 0)
			if err == nil {
				t.Errorf("OpenFile %q should have failed with path escape error", linkName)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("OpenFile %q failed with %v, expected ErrPathEscapes", linkName, err)
			}

			// Lstat should work (it doesn't follow the symlink)
			info, err := rooted.Lstat(linkName)
			if err != nil {
				t.Fatalf("Lstat %q should work: %v", linkName, err)
			}
			if info.Mode()&0o777777 == 0 { // fs.ModeSymlink
				t.Errorf("Lstat %q should show symlink mode", linkName)
			}

			// Readlink should work (it just reads the link target)
			target, err := rooted.Readlink(linkName)
			if err != nil {
				t.Fatalf("Readlink %q should work: %v", linkName, err)
			}
			if target == "" {
				t.Errorf("Readlink %q returned empty target", linkName)
			}
		})
	}
}

// test symlink resolusion escapes are allowed for unrooted.
func followSymlinkAllowedForEscapes(t *testing.T, unrooted vroot.Unrooted) {
	escapingSymlinks := []string{
		"symlink_escapes",               // -> ../../outside/outside_file.txt
		"symlink_escapes_dir",           // -> ../../outside/dir
		"subdir/symlink_upward_escapes", // -> ../symlink_escapes
	}

	for _, linkName := range escapingSymlinks {
		t.Run(linkName, func(t *testing.T) {
			// Test Lstat should work (doesn't follow symlink)
			info, err := unrooted.Lstat(linkName)
			if err != nil {
				t.Fatalf("Lstat %q failed: %v", linkName, err)
			}
			if info.Mode()&0o777777 == 0 { // fs.ModeSymlink
				t.Errorf("Lstat %q should show symlink mode", linkName)
			}

			// Test Readlink should work
			target, err := unrooted.Readlink(linkName)
			if err != nil {
				t.Fatalf("Readlink %q failed: %v", linkName, err)
			}
			if target == "" {
				t.Errorf("Readlink %q returned empty target", linkName)
			}

			// For unrooted, following symlinks that escape should be allowed
			// Note: This might fail if the target doesn't actually exist
			// In that case, we expect a "no such file" error, not ErrPathEscapes
			_, err = unrooted.Open(linkName)
			if err != nil {
				// If it fails, it should NOT be ErrPathEscapes
				if errors.Is(err, vroot.ErrPathEscapes) {
					t.Errorf("Open %q failed with ErrPathEscapes, but unrooted should allow escapes", linkName)
				}
				// Other errors (like file not found) are acceptable
			}

			_, err = unrooted.Stat(linkName)
			if err != nil {
				// If it fails, it should NOT be ErrPathEscapes
				if errors.Is(err, vroot.ErrPathEscapes) {
					t.Errorf("Stat %q failed with ErrPathEscapes, but unrooted should allow escapes", linkName)
				}
				// Other errors (like file not found) are acceptable
			}
		})
	}
}

// test path traversal.
// It fails with vroot.ErrPathEscapes
func pathTraversalFails(t *testing.T, fsys vroot.Fs) {
	traversalPaths := []string{
		"..",
		"../..",
		"../outside",
		"../outside/outside_file.txt",
		"subdir/../../outside",
		"subdir/../..",
		"./subdir/../../outside/outside_file.txt",
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			// Test Open
			_, err := fsys.Open(path)
			if err == nil {
				t.Errorf("Open %q should have failed with path traversal error", path)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Open %q failed with %v, expected ErrPathEscapes", path, err)
			}

			// Test Stat
			_, err = fsys.Stat(path)
			if err == nil {
				t.Errorf("Stat %q should have failed with path traversal error", path)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Stat %q failed with %v, expected ErrPathEscapes", path, err)
			}

			// Test Lstat
			_, err = fsys.Lstat(path)
			if err == nil {
				t.Errorf("Lstat %q should have failed with path traversal error", path)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Lstat %q failed with %v, expected ErrPathEscapes", path, err)
			}

			// Test other operations that should also fail
			err = fsys.Mkdir(path+"/newdir", 0o755)
			if err == nil {
				t.Errorf("Mkdir %q should have failed with path traversal error", path)
			} else if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Mkdir %q failed with %v, expected ErrPathEscapes", path, err)
			}

			err = fsys.Remove(path + "/somefile")
			if err == nil {
				t.Errorf("Remove %q should have failed with path traversal error", path)
			} else if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Remove %q failed with %v, expected ErrPathEscapes", path, err)
			}
		})
	}
}
