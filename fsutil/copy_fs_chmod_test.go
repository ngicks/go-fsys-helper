package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyFsOption_ChmodMask(t *testing.T) {
	type testCase struct {
		name             string
		chmodMask        fs.FileMode
		srcFileMode      fs.FileMode
		expectedFileMode fs.FileMode
		srcDirMode       fs.FileMode
		expectedDirMode  fs.FileMode
	}

	tests := []testCase{
		{
			name:             "default mask (zero)",
			chmodMask:        0,
			srcFileMode:      0o755,
			expectedFileMode: 0o755,
			srcDirMode:       0o755,
			expectedDirMode:  0o755,
		},
		{
			name:             "restrictive mask 0o755",
			chmodMask:        0o755,
			srcFileMode:      0o777,
			expectedFileMode: 0o755, // 0o777 & 0o755 = 0o755
			srcDirMode:       0o777,
			expectedDirMode:  0o755,
		},
		{
			name:             "conservative mask 0o700",
			chmodMask:        0o700,
			srcFileMode:      0o755,
			expectedFileMode: 0o700, // 0o755 & 0o700 = 0o700
			srcDirMode:       0o755,
			expectedDirMode:  0o700, // 0o755 & 0o700 = 0o700
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

			// Create copy option with ChmodMask
			opt := testCopyFsOption{ChmodMask: tc.chmodMask}

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
			if runtime.GOOS == "windows" {
				// Windows typically widens permissions
				if tc.expectedFileMode&0o200 != 0 {
					expectedFilePerm = 0o666 // read-write
				} else {
					expectedFilePerm = 0o444 // read-only
				}
			}

			if fileInfo.Mode().Perm() != expectedFilePerm {
				t.Errorf("file permissions: not equal: expected(%o) != actual(%o)", expectedFilePerm, fileInfo.Mode().Perm())
			}

			// Verify directory permissions
			dirInfo, err := os.Stat(filepath.Join(dstDir, "subdir"))
			if err != nil {
				t.Fatalf("failed to stat copied directory: %v", err)
			}

			expectedDirPerm := tc.expectedDirMode
			if runtime.GOOS == "windows" {
				// Windows typically makes directories 0o777
				expectedDirPerm = 0o777
			}

			if dirInfo.Mode().Perm() != expectedDirPerm {
				t.Errorf("directory permissions: not equal: expected(%o) != actual(%o)", expectedDirPerm, dirInfo.Mode().Perm())
			}
		})
	}
}

func TestCopyFsOption_ChmodMaskCopyPath(t *testing.T) {
	type testCase struct {
		name             string
		chmodMask        fs.FileMode
		srcFileMode      fs.FileMode
		expectedFileMode fs.FileMode
	}

	tests := []testCase{
		{
			name:             "CopyPath with restrictive mask 0o755",
			chmodMask:        0o755,
			srcFileMode:      0o777,
			expectedFileMode: 0o755, // 0o777 & 0o755 = 0o755
		},
		{
			name:             "CopyPath with permissive mask 0o777",
			chmodMask:        0o777,
			srcFileMode:      0o600,
			expectedFileMode: 0o600, // 0o600 & 0o777 = 0o600
		},
		{
			name:             "CopyPath with zero mask",
			chmodMask:        0,
			srcFileMode:      0o755,
			expectedFileMode: 0o755, // should preserve original
		},
		{
			name:             "CopyPath with conservative mask 0o700",
			chmodMask:        0o700,
			srcFileMode:      0o755,
			expectedFileMode: 0o700, // 0o755 & 0o700 = 0o700
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

			// Create copy option with ChmodMask
			opt := testCopyFsOption{ChmodMask: tc.chmodMask}

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

			expectedFilePerm := tc.expectedFileMode
			if runtime.GOOS == "windows" {
				// Windows typically widens permissions
				if tc.expectedFileMode&0o200 != 0 {
					expectedFilePerm = 0o666 // read-write
				} else {
					expectedFilePerm = 0o444 // read-only
				}
			}

			if fileInfo.Mode().Perm() != expectedFilePerm {
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
		name      string
		chmodMask fs.FileMode
		input     fs.FileMode
		expected  fs.FileMode
	}

	tests := []testCase{
		{
			name:      "zero mask uses ModePerm",
			chmodMask: 0,
			input:     0o755,
			expected:  0o755, // 0o755 & fs.ModePerm (0o777) = 0o755
		},
		{
			name:      "restrictive mask 0o644",
			chmodMask: 0o644,
			input:     0o777,
			expected:  0o644, // 0o777 & 0o644 = 0o644
		},
		{
			name:      "permissive mask 0o777",
			chmodMask: 0o777,
			input:     0o600,
			expected:  0o600, // 0o600 & 0o777 = 0o600
		},
		{
			name:      "mask removes execute bit",
			chmodMask: 0o666,
			input:     0o755,
			expected:  0o644, // 0o755 & 0o666 = 0o644 (no execute)
		},
		{
			name:      "mask removes write bit",
			chmodMask: 0o555,
			input:     0o777,
			expected:  0o555, // 0o777 & 0o555 = 0o555 (no write)
		},
		{
			name:      "very restrictive mask 0o600",
			chmodMask: 0o600,
			input:     0o755,
			expected:  0o600, // 0o755 & 0o600 = 0o600 (owner read-write only)
		},
		{
			name:      "group only mask 0o070",
			chmodMask: 0o070,
			input:     0o777,
			expected:  0o070, // 0o777 & 0o070 = 0o070 (group permissions only)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt := testCopyFsOption{ChmodMask: tc.chmodMask}
			result := opt.maskPerm(tc.input)
			if result != tc.expected {
				t.Errorf("maskPerm result: not equal: expected(%o) != actual(%o)", tc.expected, result)
			}
		})
	}
}
