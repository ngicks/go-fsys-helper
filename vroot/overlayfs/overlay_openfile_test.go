package overlayfs

import (
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

func TestOverlay_OpenFileScenarios(t *testing.T) {
	type testCase struct {
		name        func() string
		setup       func(t *testing.T, r *Fs) vroot.File
		operation   func(t *testing.T, r *Fs, f vroot.File)
		expectError bool
	}

	cases := []testCase{
		{
			name: func() string {
				return "opened file is removed - file should remain accessible"
			},
			setup: func(t *testing.T, r *Fs) vroot.File {
				// Create file in top layer
				if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
				f, err := r.top.Create("root/writable/test.txt")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Write([]byte("test content")); err != nil {
					f.Close()
					t.Fatal(err)
				}
				f.Close()

				// Open the file
				openedFile, err := r.OpenFile("root/writable/test.txt", os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				return openedFile
			},
			operation: func(t *testing.T, r *Fs, f vroot.File) {
				// Verify file exists before removal
				_, err := r.Lstat("root/writable/test.txt")
				if err != nil {
					t.Errorf("file should exist before removal: %v", err)
					return
				}

				// Remove the file while it's open
				if err := r.Remove("root/writable/test.txt"); err != nil {
					t.Errorf("failed to remove file: %v", err)
					return
				}

				// File should still be readable/writable
				buf := make([]byte, 12)
				n, err := f.Read(buf)
				if err != nil {
					t.Errorf("failed to read from opened file after removal: %v", err)
				}
				if string(buf[:n]) != "test content" {
					t.Errorf("unexpected content: %s", string(buf[:n]))
				}

				// Should be able to write to it
				if _, err := f.Seek(0, io.SeekEnd); err != nil {
					t.Errorf("failed to seek: %v", err)
				}
				if _, err := f.Write([]byte(" more")); err != nil {
					t.Errorf("failed to write to opened file after removal: %v", err)
				}
			},
			expectError: false,
		},
		{
			name: func() string {
				return "opened file parent directory is removed"
			},
			setup: func(t *testing.T, r *Fs) vroot.File {
				// Create file in top layer
				if err := r.top.MkdirAll("root/writable/subdir", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
				f, err := r.top.Create("root/writable/subdir/test.txt")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Write([]byte("test content")); err != nil {
					f.Close()
					t.Fatal(err)
				}
				f.Close()

				// Open the file
				openedFile, err := r.OpenFile("root/writable/subdir/test.txt", os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				return openedFile
			},
			operation: func(t *testing.T, r *Fs, f vroot.File) {
				// Remove parent directory while file is open
				if err := r.RemoveAll("root/writable/subdir"); err != nil {
					t.Fatalf("failed to remove directory: %v", err)
				}

				// File should still be accessible
				buf := make([]byte, 12)
				_, err := f.Read(buf)
				if err != nil {
					t.Errorf("failed to read from opened file after parent removal: %v", err)
				}
			},
			expectError: false,
		},
		{
			name: func() string {
				return "create directory where opened file exists"
			},
			setup: func(t *testing.T, r *Fs) vroot.File {
				// Create file in top layer
				if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
					t.Fatal(err)
				}
				f, err := r.top.Create("root/writable/conflict")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Write([]byte("test content")); err != nil {
					f.Close()
					t.Fatal(err)
				}
				f.Close()

				// Open the file
				openedFile, err := r.OpenFile("root/writable/conflict", os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				return openedFile
			},
			operation: func(t *testing.T, r *Fs, f vroot.File) {
				// Remove file first
				if err := r.Remove("root/writable/conflict"); err != nil {
					t.Errorf("failed to remove file: %v", err)
					return
				}

				// Try to create directory with same name
				err := r.Mkdir("root/writable/conflict", fs.ModePerm)
				if err != nil {
					t.Errorf("failed to create directory after file removal: %v", err)
				}

				// Original file should still be accessible
				buf := make([]byte, 12)
				_, err = f.Read(buf)
				if err != nil {
					t.Errorf("failed to read from opened file: %v", err)
				}
			},
			expectError: false,
		},
		{
			name: func() string {
				return "opened file from lower layer is overwritten"
			},
			setup: func(t *testing.T, r *Fs) vroot.File {
				// File should exist in lower layers from prepareLayers
				openedFile, err := r.OpenFile("root/readable/file1.txt", os.O_RDONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				return openedFile
			},
			operation: func(t *testing.T, r *Fs, f vroot.File) {
				// Ensure parent directories exist in top layer for overlay creation
				if err := r.top.MkdirAll("root/readable", fs.ModePerm); err != nil {
					t.Fatalf("failed to create parent directories: %v", err)
				}

				// Create new file with same name in top layer
				newFile, err := r.top.Create("root/readable/file1.txt")
				if err != nil {
					t.Fatalf("failed to create new file: %v", err)
				}
				if _, err := newFile.Write([]byte("new content")); err != nil {
					newFile.Close()
					t.Fatal(err)
				}
				newFile.Close()

				// Original opened file should still have old content
				buf := make([]byte, 20)
				n, err := f.Read(buf)
				if err != nil {
					t.Errorf("failed to read from original opened file: %v", err)
				}
				content := string(buf[:n])
				if content == "new content" {
					t.Errorf("opened file was affected by overlay creation")
				}
				if !strings.Contains(content, "baz") { // Original content from prepareLayers
					t.Errorf("unexpected original content: %s", content)
				}
			},
			expectError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()
			t.Logf("temp dir = %s", tempDir)

			r, closers := prepareLayers(tempDir)
			defer r.Close()
			defer closers(t)

			// Setup the test scenario
			f := tc.setup(t, r)
			defer f.Close()

			// Perform the operation
			tc.operation(t, r, f)
		})
	}
}

func TestOverlay_MultipleOpenFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	// Create a file in top layer
	if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
		t.Fatal(err)
	}
	f, err := r.top.Create("root/writable/shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("initial content")); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Open multiple handles to the same file
	f1, err := r.OpenFile("root/writable/shared.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()

	f2, err := r.OpenFile("root/writable/shared.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	// Write from one handle
	if _, err := f1.Write([]byte(" from f1")); err != nil {
		t.Errorf("failed to write from f1: %v", err)
	}

	// Write from another handle
	if _, err := f2.Write([]byte(" from f2")); err != nil {
		t.Errorf("failed to write from f2: %v", err)
	}

	// Remove file while both are open
	if err := r.Remove("root/writable/shared.txt"); err != nil {
		t.Errorf("failed to remove file with multiple open handles: %v", err)
	}

	// Both handles should still be usable
	if _, err := f1.Write([]byte(" after remove")); err != nil {
		t.Errorf("f1 became unusable after file removal: %v", err)
	}

	if _, err := f2.Write([]byte(" also after remove")); err != nil {
		t.Errorf("f2 became unusable after file removal: %v", err)
	}
}
