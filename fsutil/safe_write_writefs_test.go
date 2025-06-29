package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeWriteCopyFs(t *testing.T) {
	tempDir := t.TempDir()
	fsys := &osfsLite{base: tempDir}

	t.Run("basic copy from source fs", func(t *testing.T) {
		// Create source directory with files
		srcDir := filepath.Join(tempDir, "src")
		if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755); err != nil {
			t.Fatalf("failed to create src directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0o644); err != nil {
			t.Fatalf("failed to create src file1: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0o644); err != nil {
			t.Fatalf("failed to create src file2: %v", err)
		}

		// Create source filesystem
		srcFs := os.DirFS(srcDir)

		opt := testSafeWriteOption{}
		targetPath := "test-dir"

		err := opt.CopyFs(fsys, targetPath, srcFs, 0o755, nil, nil)
		if err != nil {
			t.Fatalf("WriteFs failed: %v", err)
		}

		// Verify files were copied
		data1, err := os.ReadFile(filepath.Join(tempDir, targetPath, "file1.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file1: %v", err)
		}
		if string(data1) != "content1" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content1", string(data1))
		}

		data2, err := os.ReadFile(filepath.Join(tempDir, targetPath, "subdir", "file2.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file2: %v", err)
		}
		if string(data2) != "content2" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content2", string(data2))
		}

		// Verify directory structure was copied
		info, err := os.Stat(filepath.Join(tempDir, targetPath, "subdir"))
		if err != nil {
			t.Fatalf("copied subdir does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("copied subdir is not a directory")
		}
	})

	t.Run("with TempFilePolicyDir", func(t *testing.T) {
		// Create source directory with files
		srcDir := filepath.Join(tempDir, "src2")
		if err := os.Mkdir(srcDir, 0o755); err != nil {
			t.Fatalf("failed to create src directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("test content"), 0o644); err != nil {
			t.Fatalf("failed to create src file: %v", err)
		}

		// Create source filesystem
		srcFs := os.DirFS(srcDir)

		tempPolicyDir := ".tmp"
		policy := newTestTempFilePolicyDir(tempPolicyDir)

		opt := testSafeWriteOption{
			TempFilePolicy: policy,
		}

		targetPath := "test-policy-dir"
		err := opt.CopyFs(fsys, targetPath, srcFs, 0o755, nil, nil)
		if err != nil {
			t.Fatalf("WriteFs failed: %v", err)
		}

		// Verify file was copied
		data, err := os.ReadFile(filepath.Join(tempDir, targetPath, "test.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file: %v", err)
		}
		if string(data) != "test content" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "test content", string(data))
		}

		// Verify temp directory was created
		if _, err := os.Stat(filepath.Join(tempDir, tempPolicyDir)); err != nil {
			t.Errorf("temp directory should exist: %v", err)
		}
	})

	t.Run("with hooks", func(t *testing.T) {
		// Create source directory with files
		srcDir := filepath.Join(tempDir, "src3")
		if err := os.Mkdir(srcDir, 0o755); err != nil {
			t.Fatalf("failed to create src directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "hooks.txt"), []byte("hooks test"), 0o644); err != nil {
			t.Fatalf("failed to create src file: %v", err)
		}

		// Create source filesystem
		srcFs := os.DirFS(srcDir)

		var hooksCalled []string

		opt := testSafeWriteOption{
			PreHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					hooksCalled = append(hooksCalled, "opt-pre")
					return nil
				},
			},
			PostHooks: []func(*os.File, string) error{
				func(f *os.File, path string) error {
					hooksCalled = append(hooksCalled, "opt-post")
					return nil
				},
			},
		}

		preHooks := []func(*os.File, string) error{
			func(f *os.File, path string) error {
				hooksCalled = append(hooksCalled, "arg-pre")
				return nil
			},
		}

		postHooks := []func(*os.File, string) error{
			func(f *os.File, path string) error {
				hooksCalled = append(hooksCalled, "arg-post")
				return nil
			},
		}

		targetPath := "test-hooks-dir"
		err := opt.CopyFs(fsys, targetPath, srcFs, 0o755, preHooks, postHooks)
		if err != nil {
			t.Fatalf("WriteFs failed: %v", err)
		}

		// Verify file was copied
		data, err := os.ReadFile(filepath.Join(tempDir, targetPath, "hooks.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file: %v", err)
		}
		if string(data) != "hooks test" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "hooks test", string(data))
		}

		// Verify hooks were called in correct order: default pre -> arg pre -> arg post -> default post
		expectedOrder := []string{"opt-pre", "arg-pre", "arg-post", "opt-post"}
		if len(hooksCalled) != len(expectedOrder) {
			t.Fatalf("not equal: expected(%d) != actual(%d)", len(expectedOrder), len(hooksCalled))
		}
		for i, v := range expectedOrder {
			if hooksCalled[i] != v {
				t.Errorf("hook order[%d]: not equal: expected(%q) != actual(%q)", i, v, hooksCalled[i])
			}
		}
	})
}
