package acceptancetest

import (
	"errors"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// test OpenRoot behavior.
// call OpenRoot against ./subdir and see now resolving symlink fails because it is now out of vroot.Rooted.
// test it is still read-only
func subRootReadOnly(t *testing.T, fsys vroot.Fs) {
	subRoot, err := fsys.OpenRoot("subdir")
	if err != nil {
		t.Fatalf("OpenRoot failed: %v", err)
	}
	defer subRoot.Close()

	// Test that the sub-root is still read-only
	_, err = subRoot.Create("should_fail.txt")
	if err == nil {
		t.Error("Create should have failed on read-only sub-root")
	}

	err = subRoot.Mkdir("should_fail_dir", 0o755)
	if err == nil {
		t.Error("Mkdir should have failed on read-only sub-root")
	}

	// Test that we can read files in the sub-root
	f, err := subRoot.Open("nested_file.txt")
	if err != nil {
		t.Errorf("Open nested_file.txt in sub-root failed: %v", err)
	} else {
		f.Close()
	}

	// Test that accessing parent directory fails (symlink_upward -> ../symlink_inner)
	// This should now fail because ../symlink_inner is outside the sub-root
	f, err = subRoot.Open("symlink_upward")
	if err == nil {
		f.Close()
		t.Errorf("Open symlink_upward should have failed with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open symlink_upward failed with %v, expected ErrPathEscapes", err)
	}

	// Test path traversal from sub-root
	f, err = subRoot.Open("..")
	if err == nil {
		f.Close()
		t.Errorf("Open .. should have failed with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open .. failed with %v, expected ErrPathEscapes", err)
	}
}

// test OpenRoot behavior.
// call OpenRoot against ./subdir and see now resolving symlink fails because it is now out of vroot.Rooted.
// test it is still read-writable.
func subRootReadWrite(t *testing.T, fsys vroot.Fs) {
	subRoot, err := fsys.OpenRoot("subdir")
	if err != nil {
		t.Fatalf("OpenRoot failed: %v", err)
	}
	defer subRoot.Close()

	// Test that the sub-root is still writable
	f, err := subRoot.Create("test_subroot.txt")
	if err != nil {
		t.Fatalf("Create should succeed on writable sub-root: %v", err)
	}
	f.Close()

	err = subRoot.Mkdir("test_subroot_dir", 0o755)
	if err != nil {
		t.Errorf("Mkdir should succeed on writable sub-root: %v", err)
	}

	// Test that we can read files in the sub-root
	f2, err := subRoot.Open("nested_file.txt")
	if err != nil {
		t.Errorf("Open nested_file.txt in sub-root failed: %v", err)
	} else {
		f2.Close()
	}

	// Test that accessing parent directory fails (symlink_upward -> ../symlink_inner)
	// This should now fail because ../symlink_inner is outside the sub-root
	f3, err := subRoot.Open("symlink_upward")
	if err == nil {
		f3.Close()
		t.Errorf("Open symlink_upward should have failed with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open symlink_upward failed with %v, expected ErrPathEscapes", err)
	}

	// Test path traversal from sub-root
	f4, err := subRoot.Open("..")
	if err == nil {
		f4.Close()
		t.Errorf("Open .. should have failed with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open .. failed with %v, expected ErrPathEscapes", err)
	}
}

// test OpenUnrooted behavior.
// call OpenUnrooted against ./subdir and see behavior. As it is [vroot.Unrooted], symlink escape is still allowed.
// test it is still read-only
func subUnrootedReadOnly(t *testing.T, fsys vroot.Unrooted) {
	subUnrooted, err := fsys.OpenUnrooted("subdir")
	if err != nil {
		t.Fatalf("OpenUnrooted failed: %v", err)
	}
	defer subUnrooted.Close()

	// Test that the sub-unrooted is still read-only
	f, err := subUnrooted.Create("should_fail.txt")
	if err == nil {
		f.Close()
		t.Error("Create should have failed on read-only sub-unrooted")
	}

	err = subUnrooted.Mkdir("should_fail_dir", 0o755)
	if err == nil {
		t.Error("Mkdir should have failed on read-only sub-unrooted")
	}

	// Test that we can read files in the sub-unrooted
	f2, err := subUnrooted.Open("nested_file.txt")
	if err != nil {
		t.Errorf("Open nested_file.txt in sub-unrooted failed: %v", err)
	} else {
		f2.Close()
	}

	// Test that accessing parent directory works (because it's unrooted)
	// symlink_upward -> ../symlink_inner should be allowed
	f3, err := subUnrooted.Open("symlink_upward")
	if err != nil {
		t.Errorf("Open symlink_upward should not fail but got %v", err)
	} else {
		f3.Close()
	}

	// Test path traversal from sub-unrooted - should be allowed but might fail if target doesn't exist
	f4, err := subUnrooted.Open("..")
	if err == nil {
		f4.Close()
		t.Errorf("Open .. should fail with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open .. should fail with ErrPathEscapes but got %v", err)
	}
}

// test OpenUnrooted behavior.
// call OpenUnrooted against ./subdir and see behavior. As it is [vroot.Unrooted], symlink escape is still allowed.
// test it is still read-writable.
func subUnrootedReadWrite(t *testing.T, fsys vroot.Fs) {
	// This function signature should probably take vroot.Unrooted, but following the existing signature
	unrooted, ok := fsys.(vroot.Unrooted)
	if !ok {
		t.Skip("fsys is not Unrooted, skipping test")
		return
	}

	subUnrooted, err := unrooted.OpenUnrooted("subdir")
	if err != nil {
		t.Fatalf("OpenUnrooted failed: %v", err)
	}
	defer subUnrooted.Close()

	// Test that the sub-unrooted is still writable
	f, err := subUnrooted.Create("test_subunrooted.txt")
	if err != nil {
		t.Fatalf("Create should succeed on writable sub-unrooted: %v", err)
	}
	f.Close()

	err = subUnrooted.Mkdir("test_subunrooted_dir", 0o755)
	if err != nil {
		t.Errorf("Mkdir should succeed on writable sub-unrooted: %v", err)
	}

	// Test that we can read files in the sub-unrooted
	f, err = subUnrooted.Open("nested_file.txt")
	if err != nil {
		t.Errorf("Open nested_file.txt in sub-unrooted failed: %v", err)
	} else {
		f.Close()
	}

	// Test that accessing parent directory works (because it's unrooted)
	// symlink_upward -> ../symlink_inner should be allowed
	f, err = subUnrooted.Open("symlink_upward")
	if err != nil {
		t.Errorf("Open symlink_upward should not fail in unrooted: %v", err)
	} else {
		f.Close()
	}

	// Test path traversal from sub-unrooted - should be allowed but might fail if target doesn't exist
	f, err = subUnrooted.Open("..")
	if err == nil {
		f.Close()
		t.Errorf("Open .. should fail with ErrPathEscapes")
	} else if !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("Open .. should fail with ErrPathEscapes but got %v", err)
	}
}
