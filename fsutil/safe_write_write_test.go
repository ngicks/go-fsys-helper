package fsutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

// checkNoTempFiles verifies no temporary files are left in the directory
func checkNoTempFiles(t *testing.T, tempDir string) {
	t.Helper()
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp directory: %v", err)
	}

	policy := testTempFilePolicyRandom{}
	for _, entry := range entries {
		if policy.Match(entry.Name()) {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestSafeWrite(t *testing.T) {
	t.Run("basic copy from reader", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{}
		targetPath := "test.txt"
		content := "hello world"

		err := opt.Copy(fsys, targetPath, strings.NewReader(content), 0o644, nil, nil)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify file was created with correct content
		data, err := os.ReadFile(filepath.Join(tempDir, targetPath))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != content {
			t.Errorf("not equal: expected(%q) != actual(%q)", content, string(data))
		}

		// Verify permissions
		info, err := os.Stat(filepath.Join(tempDir, targetPath))
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		expectedPerm := fs.FileMode(0o644)
		if runtime.GOOS == "windows" {
			expectedPerm = 0o666 // Windows typically widens to read-write
		}
		if info.Mode().Perm() != expectedPerm {
			t.Errorf("not equal: expected(%o) != actual(%o)", expectedPerm, info.Mode().Perm())
		}
	})

	t.Run("copy with pre and post hooks", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		var preHookCalled, postHookCalled, optPreHookCalled, optPostHookCalled bool
		var hookOrder []string

		opt := testSafeWriteOption{
			PreHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					optPreHookCalled = true
					hookOrder = append(hookOrder, "opt-pre")
					return nil
				},
			},
			PostHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					optPostHookCalled = true
					hookOrder = append(hookOrder, "opt-post")
					return nil
				},
			},
		}

		preHooks := []func(*os.File, string) error{
			func(f *os.File, path string) error {
				preHookCalled = true
				hookOrder = append(hookOrder, "arg-pre")
				return nil
			},
		}

		postHooks := []func(*os.File, string) error{
			func(f *os.File, path string) error {
				postHookCalled = true
				hookOrder = append(hookOrder, "arg-post")
				return nil
			},
		}

		targetPath := "test-hooks.txt"
		err := opt.Copy(fsys, targetPath, strings.NewReader("content"), 0o644, preHooks, postHooks)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify all hooks were called
		if !preHookCalled || !postHookCalled || !optPreHookCalled || !optPostHookCalled {
			t.Error("not all hooks were called")
		}

		// Verify hook execution order: default pre -> arg pre -> arg post -> default post
		expectedOrder := []string{"opt-pre", "arg-pre", "arg-post", "opt-post"}
		if len(hookOrder) != len(expectedOrder) {
			t.Fatalf("not equal: expected(%d) != actual(%d)", len(expectedOrder), len(hookOrder))
		}
		for i, v := range expectedOrder {
			if hookOrder[i] != v {
				t.Errorf("hook order[%d]: not equal: expected(%q) != actual(%q)", i, v, hookOrder[i])
			}
		}
	})

	t.Run("copy with sync hook", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{
			PostHooks: []func(*os.File, string) error{
				SyncHook[*os.File],
			},
		}

		targetPath := "test-sync.txt"
		err := opt.Copy(fsys, targetPath, strings.NewReader("synced content"), 0o644, nil, nil)
		if err != nil {
			t.Fatalf("Write with sync hook failed: %v", err)
		}
	})

	t.Run("copy error in pre-hook", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		hookErr := errors.New("pre-hook error")
		opt := testSafeWriteOption{
			PreHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					return hookErr
				},
			},
		}

		targetPath := "test-pre-error.txt"
		err := opt.Copy(fsys, targetPath, strings.NewReader("content"), 0o644, nil, nil)
		if err != hookErr {
			t.Errorf("errors.Is(err, %v) does not satisfied:\nactual = %v\ndetailed = %#v", hookErr, err, err)
		}

		// Verify file was not created
		if _, err := os.Stat(filepath.Join(tempDir, targetPath)); !errors.Is(err, fs.ErrNotExist) {
			t.Error("file should not exist after pre-hook error")
		}

		// Verify no temporary files are left behind
		checkNoTempFiles(t, tempDir)
	})

	t.Run("copy error in post-hook", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		hookErr := errors.New("post-hook error")
		opt := testSafeWriteOption{
			PostHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					return hookErr
				},
			},
		}

		targetPath := "test-post-error.txt"
		err := opt.Copy(fsys, targetPath, strings.NewReader("content"), 0o644, nil, nil)
		if err != hookErr {
			t.Errorf("errors.Is(err, %v) does not satisfied:\nactual = %v\ndetailed = %#v", hookErr, err, err)
		}

		// Verify file was not created
		if _, err := os.Stat(filepath.Join(tempDir, targetPath)); !errors.Is(err, fs.ErrNotExist) {
			t.Error("file should not exist after post-hook error")
		}

		// Verify no temporary files are left behind
		checkNoTempFiles(t, tempDir)
	})

	t.Run("copy with read error", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{}
		targetPath := "test-write-error.txt"

		// Reader that always errors
		errReader := &errorReader{err: io.ErrUnexpectedEOF}

		err := opt.Copy(fsys, targetPath, errReader, 0o644, nil, nil)
		if err != io.ErrUnexpectedEOF {
			t.Errorf("errors.Is(err, %v) does not satisfied:\nactual = %v\ndetailed = %#v", io.ErrUnexpectedEOF, err, err)
		}

		// Verify file was not created
		if _, err := os.Stat(filepath.Join(tempDir, targetPath)); !errors.Is(err, fs.ErrNotExist) {
			t.Error("file should not exist after write error")
		}

		// Verify no temporary files are left behind
		checkNoTempFiles(t, tempDir)
	})

	t.Run("copy overwrite existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{}
		targetPath := "test-overwrite.txt"

		// Create initial file
		if err := os.WriteFile(filepath.Join(tempDir, targetPath), []byte("old content"), 0o644); err != nil {
			t.Fatalf("failed to create initial file: %v", err)
		}

		// Overwrite with SafeWrite
		newContent := "new content"
		err := opt.Copy(fsys, targetPath, strings.NewReader(newContent), 0o644, nil, nil)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify content was overwritten
		data, err := os.ReadFile(filepath.Join(tempDir, targetPath))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != newContent {
			t.Errorf("not equal: expected(%q) != actual(%q)", newContent, string(data))
		}
	})

	t.Run("copy with TempFilePolicyDir", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		tempPolicyDir := ".tmp"
		policy := newTestTempFilePolicyDir(tempPolicyDir)

		opt := testSafeWriteOption{
			TempFilePolicy: policy,
		}

		targetPath := "test-policy-dir.txt"
		err := opt.Copy(fsys, targetPath, strings.NewReader("content"), 0o644, nil, nil)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify temp directory was created
		if _, err := os.Stat(filepath.Join(tempDir, tempPolicyDir)); err != nil {
			t.Errorf("temp directory should exist: %v", err)
		}

		// Clean up temp files using WalkFunc
		wrapped := &testFsysWrapper{fsys: fsys}
		err = fs.WalkDir(wrapped, ".", func(path string, d fs.DirEntry, err error) error {
			return policy.WalkFunc(fsys, path, d, err)
		})
		if err != nil {
			t.Errorf("WalkFunc failed: %v", err)
		}
	})

	t.Run("Write method with writer function", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{}
		targetPath := "test-write-func.txt"
		content := "content written via writer function"

		err := opt.Write(fsys, targetPath, func(w io.Writer) error {
			_, err := w.Write([]byte(content))
			return err
		}, 0o644, nil, nil)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Verify file was created with correct content
		data, err := os.ReadFile(filepath.Join(tempDir, targetPath))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != content {
			t.Errorf("not equal: expected(%q) != actual(%q)", content, string(data))
		}

		// Verify permissions
		info, err := os.Stat(filepath.Join(tempDir, targetPath))
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		expected := 0o644
		if runtime.GOOS == "windows" {
			expected = 0o666
		}
		if info.Mode().Perm() != fs.FileMode(expected) {
			t.Errorf("wrong permissions: expected 0o644, got %o", info.Mode().Perm())
		}
	})

	t.Run("Write method with error in writer function", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := osfslite.New(tempDir)

		opt := testSafeWriteOption{}
		targetPath := "test-write-error.txt"
		expectedErr := errors.New("write error")

		err := opt.Write(fsys, targetPath, func(w io.Writer) error {
			return expectedErr
		}, 0o644, nil, nil)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		// Verify file was not created
		if _, err := os.Stat(filepath.Join(tempDir, targetPath)); err == nil {
			t.Error("file should not exist when write function returns error")
		}

		// Verify no temporary files are left behind
		checkNoTempFiles(t, tempDir)
	})
}
