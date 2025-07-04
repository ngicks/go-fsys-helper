package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyFsOption_MaskChmodMode(t *testing.T) {
	type testCase struct {
		name             string
		maskFunc         func(fs.FileMode) fs.FileMode
		srcFileMode      fs.FileMode
		expectedFileMode fs.FileMode
		srcDirMode       fs.FileMode
		expectedDirMode  fs.FileMode
	}

	nonWindowsOr := func(l, r fs.FileMode) fs.FileMode {
		if runtime.GOOS != "windows" {
			return l
		}
		return r
	}
	tests := []testCase{
		{
			name:             "default behavior (nil function)",
			maskFunc:         nil,
			srcFileMode:      0o755,
			expectedFileMode: nonWindowsOr(0o755, 0o666),
			srcDirMode:       0o755,
			expectedDirMode:  nonWindowsOr(0o755, 0o777),
		},
		{
			name: "restrictive mask 0o755",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o755
			},
			srcFileMode:      0o777,
			expectedFileMode: nonWindowsOr(0o755, 0o666),
			srcDirMode:       0o777,
			expectedDirMode:  nonWindowsOr(0o755, 0o777),
		},
		{
			name: "conservative mask 0o700",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o700
			},
			srcFileMode:      0o755,
			expectedFileMode: nonWindowsOr(0o700, 0o666),
			srcDirMode:       0o755,
			expectedDirMode:  nonWindowsOr(0o700, 0o777),
		},
		{
			name:             "using platform-specific MaskChmodMode",
			maskFunc:         MaskChmodMode,
			srcFileMode:      0o755,
			expectedFileMode: nonWindowsOr(0o755, 0o666),
			srcDirMode:       0o755,
			expectedDirMode:  nonWindowsOr(0o755, 0o777), // Will be adjusted per platform below
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create root directory
			tempDir := t.TempDir()
			srcDir := filepath.Join(tempDir, "src")
			dstDir := filepath.Join(tempDir, "dst")

			// Create src and dst directories
			if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
				t.Fatalf("failed to create src dir: %v", err)
			}
			if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
				t.Fatalf("failed to create dst dir: %v", err)
			}

			// Create source subdirectory and file with specific permissions
			subDir := filepath.Join(srcDir, "subdir")
			if err := os.Mkdir(subDir, tc.srcDirMode); err != nil {
				t.Fatalf("failed to create source subdir: %v", err)
			}

			srcFile := filepath.Join(srcDir, "testfile.txt")
			if err := os.WriteFile(srcFile, []byte("test content"), tc.srcFileMode); err != nil {
				t.Fatalf("failed to create source file: %v", err)
			}

			// Set up filesystems
			srcFs := os.DirFS(srcDir)
			dstFs := &osfsLite{base: dstDir}

			// Create copy option with MaskChmodMode
			opt := testCopyFsOption{MaskChmodMode: tc.maskFunc}

			// Perform copy
			err := opt.CopyAll(dstFs, srcFs, ".")
			if err != nil {
				t.Fatalf("Copy failed: %v", err)
			}

			// Verify file permissions
			fileInfo, err := os.Stat(filepath.Join(dstDir, "testfile.txt"))
			if err != nil {
				t.Fatalf("failed to stat copied file: %v", err)
			}

			expectedFilePerm := tc.expectedFileMode
			if fileInfo.Mode().Perm() != expectedFilePerm {
				t.Errorf("file permissions: not equal: expected(%o) != actual(%o)", expectedFilePerm, fileInfo.Mode().Perm())
			}

			// Verify directory permissions
			dirInfo, err := os.Stat(filepath.Join(dstDir, "subdir"))
			if err != nil {
				t.Fatalf("failed to stat copied directory: %v", err)
			}

			expectedDirPerm := tc.expectedDirMode
			if dirInfo.Mode().Perm() != expectedDirPerm {
				t.Errorf("directory permissions: not equal: expected(%o) != actual(%o)", expectedDirPerm, dirInfo.Mode().Perm())
			}
		})
	}
}

func TestCopyFsOption_MaskChmodModeCopyPath(t *testing.T) {
	type testCase struct {
		name             string
		maskFunc         func(fs.FileMode) fs.FileMode
		srcFileMode      fs.FileMode
		expectedFileMode fs.FileMode
	}

	tests := []testCase{
		{
			name: "CopyPath with restrictive mask 0o755",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o755
			},
			srcFileMode:      0o777,
			expectedFileMode: 0o755, // 0o777 & 0o755 = 0o755
		},
		{
			name: "CopyPath with permissive mask 0o777",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o777
			},
			srcFileMode:      0o600,
			expectedFileMode: 0o600, // 0o600 & 0o777 = 0o600
		},
		{
			name:             "CopyPath with nil mask",
			maskFunc:         nil,
			srcFileMode:      0o755,
			expectedFileMode: 0o755, // should preserve original
		},
		{
			name: "CopyPath with conservative mask 0o700",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o700
			},
			srcFileMode:      0o755,
			expectedFileMode: 0o700, // 0o755 & 0o700 = 0o700
		},
		{
			name:             "CopyPath with platform-specific MaskChmodMode",
			maskFunc:         MaskChmodMode,
			srcFileMode:      0o755,
			expectedFileMode: 0o755, // Will be adjusted per platform below
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create root directory
			tempDir := t.TempDir()
			srcDir := filepath.Join(tempDir, "src")
			dstDir := filepath.Join(tempDir, "dst")

			// Create src and dst directories
			if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
				t.Fatalf("failed to create src dir: %v", err)
			}
			if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
				t.Fatalf("failed to create dst dir: %v", err)
			}

			// Create source file in subdirectory
			if err := os.MkdirAll(filepath.Join(srcDir, "nested"), fs.ModePerm); err != nil {
				t.Fatalf("failed to create source nested dir: %v", err)
			}

			srcFile := filepath.Join(srcDir, "nested", "testfile.txt")
			if err := os.WriteFile(srcFile, []byte("test content"), tc.srcFileMode); err != nil {
				t.Fatalf("failed to create source file: %v", err)
			}

			// Set up filesystems
			srcFs := os.DirFS(srcDir)
			dstFs := &osfsLite{base: dstDir}

			// Create copy option with MaskChmodMode
			opt := testCopyFsOption{MaskChmodMode: tc.maskFunc}

			// Perform copy using CopyPath
			err := opt.CopyPath(dstFs, srcFs, ".", filepath.FromSlash("nested/testfile.txt"))
			if err != nil {
				t.Fatalf("CopyPath failed: %v", err)
			}

			// Verify file permissions
			fileInfo, err := os.Stat(filepath.Join(dstDir, "nested", "testfile.txt"))
			if err != nil {
				t.Fatalf("failed to stat copied file: %v", err)
			}

			expectedFilePerm := MaskChmodMode(tc.expectedFileMode)
			if fileInfo.Mode().Perm() != (expectedFilePerm) {
				t.Errorf("file permissions: not equal: expected(%o) != actual(%o)", expectedFilePerm, fileInfo.Mode().Perm())
			}

			// Verify that the nested directory was created and has appropriate permissions
			dirInfo, err := os.Stat(filepath.Join(dstDir, "nested"))
			if err != nil {
				t.Fatalf("failed to stat copied directory: %v", err)
			}
			if !dirInfo.IsDir() {
				t.Error("nested should be a directory")
			}
		})
	}
}

func TestCopyFsOption_maskPerm(t *testing.T) {
	type testCase struct {
		name     string
		maskFunc func(fs.FileMode) fs.FileMode
		input    fs.FileMode
		expected fs.FileMode
	}

	tests := []testCase{
		{
			name:     "nil function uses ModePerm",
			maskFunc: nil,
			input:    0o755,
			expected: 0o755, // 0o755 & fs.ModePerm (0o777) = 0o755
		},
		{
			name: "restrictive mask 0o644",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o644
			},
			input:    0o777,
			expected: 0o644, // 0o777 & 0o644 = 0o644
		},
		{
			name: "permissive mask 0o777",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o777
			},
			input:    0o600,
			expected: 0o600, // 0o600 & 0o777 = 0o600
		},
		{
			name: "mask removes execute bit",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o666
			},
			input:    0o755,
			expected: 0o644, // 0o755 & 0o666 = 0o644 (no execute)
		},
		{
			name: "mask removes write bit",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o555
			},
			input:    0o777,
			expected: 0o555, // 0o777 & 0o555 = 0o555 (no write)
		},
		{
			name: "very restrictive mask 0o600",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o600
			},
			input:    0o755,
			expected: 0o600, // 0o755 & 0o600 = 0o600 (owner read-write only)
		},
		{
			name: "group only mask 0o070",
			maskFunc: func(perm fs.FileMode) fs.FileMode {
				return perm & 0o070
			},
			input:    0o777,
			expected: 0o070, // 0o777 & 0o070 = 0o070 (group permissions only)
		},
		{
			name:     "using platform-specific MaskChmodMode",
			maskFunc: MaskChmodMode,
			input:    0o755 | os.ModeSetuid,
			expected: 0o755, // Will be adjusted based on platform
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt := testCopyFsOption{MaskChmodMode: tc.maskFunc}
			result := opt.maskPerm(tc.input)

			expected := tc.expected
			// Adjust expectation for platform-specific MaskChmodMode
			if tc.name == "using platform-specific MaskChmodMode" {
				expected = MaskChmodMode(tc.input)
			}

			if result != expected {
				t.Errorf("maskPerm result: not equal: expected(%o) != actual(%o)", expected, result)
			}
		})
	}
}
