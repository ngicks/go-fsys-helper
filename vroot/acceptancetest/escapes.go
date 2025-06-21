package acceptancetest

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var escapingSymlinks = []string{ // pairs of link name and target.
	"symlink_escapes", filepath.FromSlash("../../outside/outside_file.txt"),
	"symlink_escapes_dir", filepath.FromSlash("../../outside/dir"),
	filepath.FromSlash("subdir/symlink_upward_escapes"), filepath.FromSlash("../symlink_escapes"),
}

// test symlink resolusion fails if it escapes the root.
// call Open or similar methods and check if it fails with [vroot.ErrPathEscapes]
func followSymlinkFailsForEscapes(t *testing.T, rooted vroot.Rooted) {
	for linkNameAndTarget := range slices.Chunk(escapingSymlinks, 2) {
		linkName := linkNameAndTarget[0]
		expectedTarget := linkNameAndTarget[1]
		t.Run(linkName, func(t *testing.T) {
			// Test Open should fail with ErrPathEscapes
			f, err := rooted.Open(linkName)
			if err == nil {
				f.Close()
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
			f, err = rooted.OpenFile(linkName, 0, 0)
			if err == nil {
				f.Close()
				t.Errorf("OpenFile %q should have failed with path escape error", linkName)
				return
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("OpenFile %q failed with %v, expected ErrPathEscapes", linkName, err)
			}

			// Lstat should work (it doesn't follow the symlink)
			info, err := rooted.Lstat(linkName)
			if err != nil {
				t.Errorf("Lstat %q should work: %v", linkName, err)
			}

			if info != nil && info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("Lstat %q should show symlink mode", linkName)
			}

			// ReadLink should work (it just reads the link target)
			target, err := rooted.ReadLink(linkName)
			if err != nil {
				t.Errorf("ReadLink %q should work: %v", linkName, err)
			}

			if target != expectedTarget {
				t.Errorf("ReadLink %q returned invalid value: want(%q) != got(%q)", linkName, expectedTarget, target)
			}
		})
	}
}

// test symlink resolusion escapes are allowed for unrooted.
func followSymlinkAllowedForEscapes(t *testing.T, unrooted vroot.Unrooted, hasOutside bool) {
	for linkNameAndTarget := range slices.Chunk(escapingSymlinks, 2) {
		linkName := linkNameAndTarget[0]
		expectedTarget := linkNameAndTarget[1]
		t.Run(linkName, func(t *testing.T) {
			info, err := unrooted.Lstat(linkName)
			if err != nil {
				t.Errorf("Lstat %q failed: %v", linkName, err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("Lstat %q should show symlink mode", linkName)
			}

			target, err := unrooted.ReadLink(linkName)
			if err != nil {
				t.Fatalf("ReadLink %q failed: %v", linkName, err)
			}
			if target != expectedTarget {
				t.Errorf("ReadLink %q returned invalid value: want(%q) != got(%q)", linkName, expectedTarget, target)
			}

			// For unrooted, following symlinks that escape should be allowed
			// Note: This might fail if the target doesn't actually exist
			// In that case, we expect a "no such file" error, not ErrPathEscapes
			f, err := unrooted.Open(linkName)
			if err == nil {
				f.Close()
			}
			if hasOutside && err != nil {
				t.Errorf("Open %q failed with %v", linkName, err)
			} else if !hasOutside && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Open %q should have failed with error since the fsys does not have outside", linkName)
			}

			_, err = unrooted.Stat(linkName)
			if hasOutside && err != nil {
				t.Errorf("Stat %q failed with %v", linkName, err)
			} else if !hasOutside && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Stat %q should have failed with error since the fsys does not have outside", linkName)
			}
		})
	}
}

// test path traversal.
// It fails with vroot.ErrPathEscapes or for read-only implementations with EROFS/EPERM
func pathTraversalFails(t *testing.T, fsys vroot.Fs, isReadOnly bool) {
	traversalPaths := []string{
		"..",
		filepath.FromSlash("../.."),
		filepath.FromSlash("../outside"),
		filepath.FromSlash("../outside/outside_file.txt"),
		filepath.FromSlash("subdir/../../outside"),
		filepath.FromSlash("subdir/../.."),
		filepath.FromSlash("./subdir/../../outside/outside_file.txt"),
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			// Test Open
			f, err := fsys.Open(path)
			if err == nil {
				f.Close()
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
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Lstat %q failed with %v, expected ErrPathEscapes", path, err)
			}

			// Test other operations that should also fail
			err = fsys.Mkdir(filepath.Join(path, "newdir"), 0o755)
			if err == nil {
				t.Errorf("Mkdir %q should have failed with path traversal error", path)
			} else if !errors.Is(err, vroot.ErrPathEscapes) {
				if isReadOnly && (errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EPERM)) {
					// Accept EROFS/EPERM for read-only implementations
				} else {
					t.Errorf("Mkdir %q failed with %v, expected ErrPathEscapes", path, err)
				}
			}

			err = fsys.Remove(filepath.Join(path, "somefile"))
			if err == nil {
				t.Errorf("Remove %q should have failed with path traversal error", path)
			} else if !errors.Is(err, vroot.ErrPathEscapes) {
				if isReadOnly && (errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EPERM)) {
					// Accept EROFS/EPERM for read-only implementations
				} else {
					t.Errorf("Remove %q failed with %v, expected ErrPathEscapes", path, err)
				}
			}
		})
	}
}
