package overlay

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOverlay_FileDirectoryConflicts(t *testing.T) {
	type testCase struct {
		name      func() string
		setup     func(t *testing.T, r *Overlay)
		operation func(t *testing.T, r *Overlay) error
		check     func(t *testing.T, r *Overlay, err error)
	}

	cases := []testCase{
		{
			name: func() string {
				return "create file where directory exists in layer"
			},
			setup: func(t *testing.T, r *Overlay) {
				// Directory exists in lower layer at root/readable/subdir
			},
			operation: func(t *testing.T, r *Overlay) error {
				return r.top.MkdirAll(filepath.Dir("root/readable/subdir"), fs.ModePerm)
			},
			check: func(t *testing.T, r *Overlay, err error) {
				// Try to create file with same name as directory
				f, createErr := r.Create("root/readable/subdir")
				if createErr == nil {
					f.Close()
					t.Error("expected error when creating file over directory")
				}
				if !errors.Is(createErr, fs.ErrExist) {
					t.Errorf("expected fs.ErrExist, got %v", createErr)
				}
			},
		},
		{
			name: func() string {
				return "create directory where file exists in layer"
			},
			setup: func(t *testing.T, r *Overlay) {
				// File exists in lower layer at root/readable/file1.txt
			},
			operation: func(t *testing.T, r *Overlay) error {
				return nil // No setup operation needed
			},
			check: func(t *testing.T, r *Overlay, err error) {
				// Try to create directory with same name as file
				mkdirErr := r.Mkdir("root/readable/file1.txt", fs.ModePerm)
				if mkdirErr == nil {
					t.Error("expected error when creating directory over file")
				}
				// Should get some kind of conflict error
			},
		},
		{
			name: func() string {
				return "remove directory and create file with same name"
			},
			setup: func(t *testing.T, r *Overlay) {
				// Create directory in top layer
				if err := r.top.MkdirAll("root/writable/conflict", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
			},
			operation: func(t *testing.T, r *Overlay) error {
				return r.RemoveAll("root/writable/conflict")
			},
			check: func(t *testing.T, r *Overlay, err error) {
				if err != nil {
					t.Errorf("failed to remove directory: %v", err)
				}

				// Should be able to create file with same name
				f, createErr := r.Create("root/writable/conflict")
				if createErr != nil {
					t.Errorf("failed to create file after directory removal: %v", createErr)
				} else {
					f.Close()
				}

				// Verify it's a file
				info, statErr := r.Lstat("root/writable/conflict")
				if statErr != nil {
					t.Errorf("failed to stat created file: %v", statErr)
				} else if info.IsDir() {
					t.Error("expected file, got directory")
				}
			},
		},
		{
			name: func() string {
				return "remove file and create directory with same name"
			},
			setup: func(t *testing.T, r *Overlay) {
				// Create file in top layer
				if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
				f, err := r.top.Create("root/writable/conflict")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()
			},
			operation: func(t *testing.T, r *Overlay) error {
				return r.Remove("root/writable/conflict")
			},
			check: func(t *testing.T, r *Overlay, err error) {
				if err != nil {
					t.Errorf("failed to remove file: %v", err)
				}

				// Should be able to create directory with same name
				mkdirErr := r.Mkdir("root/writable/conflict", fs.ModePerm)
				if mkdirErr != nil {
					t.Errorf("failed to create directory after file removal: %v", mkdirErr)
				}

				// Verify it's a directory
				info, statErr := r.Lstat("root/writable/conflict")
				if statErr != nil {
					t.Errorf("failed to stat created directory: %v", statErr)
				} else if !info.IsDir() {
					t.Error("expected directory, got file")
				}
			},
		},
		{
			name: func() string {
				return "symlink pointing to file replaced by directory"
			},
			setup: func(t *testing.T, r *Overlay) {
				// Create target file and symlink in top layer
				if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
				f, err := r.top.Create("root/writable/target")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()

				if err := r.Symlink("target", "root/writable/link"); err != nil {
					t.Fatal(err)
				}
			},
			operation: func(t *testing.T, r *Overlay) error {
				// Remove target and replace with directory
				if err := r.Remove("root/writable/target"); err != nil {
					return err
				}
				return r.Mkdir("root/writable/target", fs.ModePerm)
			},
			check: func(t *testing.T, r *Overlay, err error) {
				if err != nil {
					t.Errorf("failed operation: %v", err)
				}

				// Symlink should now point to directory
				info, statErr := r.Stat("root/writable/link") // Stat follows symlinks
				if statErr != nil {
					t.Errorf("failed to stat symlink target: %v", statErr)
				} else if !info.IsDir() {
					t.Error("symlink should now point to directory")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()
			t.Logf("temp dir = %s", tempDir)

			r, closers := prepareLayers(tempDir)
			defer r.Close()
			defer closers(t)

			tc.setup(t, r)
			err := tc.operation(t, r)
			tc.check(t, r, err)
		})
	}
}

func TestOverlay_CopyOnWriteConflicts(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	// Test copy-on-write with concurrent operations
	t.Run("copy-on-write during file modification", func(t *testing.T) {
		// File exists in lower layer - root/readable/file1.txt

		// Open for writing - should trigger copy-on-write
		f, err := r.OpenFile("root/readable/file1.txt", os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("failed to open file: %v", err)
		}
		defer f.Close()

		// Write to it - this should modify the top layer copy
		_, err = f.Write([]byte(" modified"))
		if err != nil {
			t.Errorf("failed to write to file: %v", err)
		}

		// Verify original file in lower layer is unchanged by opening read-only
		// and checking it doesn't contain our modification
		// (This is difficult to test directly, but we can verify the overlay behavior)

		// File should now exist in top layer
		_, err = r.top.Lstat("root/readable/file1.txt")
		if err != nil {
			t.Errorf("file should exist in top layer after copy-on-write: %v", err)
		}
	})

	t.Run("copy-on-write directory structure", func(t *testing.T) {
		// File exists in nested directory in lower layer
		filePath := "root/readable/subdir/nested_file.txt"

		// Modify the nested file
		f, err := r.OpenFile(filePath, os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("failed to open nested file: %v", err)
		}
		defer f.Close()

		_, err = f.Write([]byte(" modified"))
		if err != nil {
			t.Errorf("failed to write to nested file: %v", err)
		}

		// Parent directories should be created in top layer
		_, err = r.top.Lstat("root/readable")
		if err != nil {
			t.Errorf("parent directory should exist in top layer: %v", err)
		}

		_, err = r.top.Lstat("root/readable/subdir")
		if err != nil {
			t.Errorf("nested parent directory should exist in top layer: %v", err)
		}

		_, err = r.top.Lstat(filePath)
		if err != nil {
			t.Errorf("file should exist in top layer: %v", err)
		}
	})
}

func TestOverlay_WhiteoutBehavior(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	t.Run("remove file from lower layer creates whiteout", func(t *testing.T) {
		// File exists in lower layer
		_, err := r.Lstat("root/readable/file1.txt")
		if err != nil {
			t.Fatalf("file should exist: %v", err)
		}

		// Remove it
		err = r.Remove("root/readable/file1.txt")
		if err != nil {
			t.Fatalf("failed to remove file: %v", err)
		}

		// File should no longer be visible
		_, err = r.Lstat("root/readable/file1.txt")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("removed file should not exist, got error: %v", err)
		}

		// But whiteout should be recorded
		whited, err := r.topMeta.QueryWhiteout("root/readable/file1.txt")
		if err != nil {
			t.Errorf("failed to query whiteout: %v", err)
		}
		if !whited {
			t.Error("whiteout should be recorded for removed file")
		}
	})

	t.Run("create file over whited out file", func(t *testing.T) {
		// Remove a file to create whiteout
		err := r.Remove("root/readable/file2.txt")
		if err != nil {
			t.Fatalf("failed to remove file: %v", err)
		}

		// Verify it's whited out
		whited, err := r.topMeta.QueryWhiteout("root/readable/file2.txt")
		if err != nil {
			t.Fatalf("failed to query whiteout: %v", err)
		}
		if !whited {
			t.Fatal("file should be whited out")
		}

		// Create new file with same name
		f, err := r.Create("root/readable/file2.txt")
		if err != nil {
			t.Fatalf("failed to create file over whiteout: %v", err)
		}
		_, err = f.Write([]byte("new content"))
		if err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()

		// File should be visible again
		_, err = r.Lstat("root/readable/file2.txt")
		if err != nil {
			t.Errorf("created file should be visible: %v", err)
		}

		// Whiteout should be cleared
		whited, err = r.topMeta.QueryWhiteout("root/readable/file2.txt")
		if err != nil {
			t.Errorf("failed to query whiteout after creation: %v", err)
		}
		if whited {
			t.Error("whiteout should be cleared after file creation")
		}
	})
}
