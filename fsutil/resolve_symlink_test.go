//go:build !plan9

package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"testing"
)

type toAndFro struct {
	to, from string
}

type resolvePathTestCase struct {
	name     func() string
	links    []toAndFro
	from     string
	file     string // empty file created at there.
	resolved string
	err      error
}

func makeChainedSymlink(baseDir string, num int) iter.Seq[toAndFro] {
	return func(yield func(toAndFro) bool) {
		for i := range num {
			if !yield(toAndFro{
				fmt.Sprintf("%d", i+1),
				fmt.Sprintf("./%s/%d", baseDir, i),
			}) {
				return
			}
		}
	}
}

func TestResolvePath(t *testing.T) {
	cases := []resolvePathTestCase{
		{
			func() string {
				return "normal resolution"
			},
			[]toAndFro{
				{"a", "./foo/b"},
				{"./c/d", "./foo/a"},
				{"../bar", "./foo/c/d"},
			},
			"./foo/b",
			"./foo/bar",
			"foo/bar",
			nil,
		},
		{
			func() string {
				return "no entry from intermediate result"
			},
			[]toAndFro{
				{"bar", "foo/a"},
				{"", "./foo/bar/baz"},
			},
			"./foo/a/b/c/d",
			"",
			"foo/bar/b/c/d",
			fs.ErrNotExist,
		},
		{
			func() string {
				return "many link"
			},
			slices.Collect(makeChainedSymlink("foo", maxSymlinkResolutionCount-1)),
			"./foo/0",
			fmt.Sprintf("./foo/%d", maxSymlinkResolutionCount-1),
			fmt.Sprintf("foo/%d", maxSymlinkResolutionCount-1),
			nil,
		},
		{
			func() string {
				return "too many link"
			},
			slices.Collect(makeChainedSymlink("foo", maxSymlinkResolutionCount)),
			"./foo/0",
			fmt.Sprintf("./foo/%d", maxSymlinkResolutionCount),
			"",
			syscall.ELOOP,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()

			t.Logf("tempDir = %s", tempDir)

			if tc.file != "" {
				filePath := filepath.Join(
					tempDir,
					filepath.FromSlash(tc.file),
				)
				err := os.MkdirAll(
					filepath.Dir(filePath),
					fs.ModePerm,
				)
				if err != nil {
					panic(err)
				}

				f, err := os.Create(filePath)
				if err != nil {
					panic(err)
				}
				_, err = f.WriteString("foobarbaz\n")
				_ = f.Close()
				if err != nil {
					panic(err)
				}
			}

			for _, link := range tc.links {
				err := os.MkdirAll(
					filepath.Dir(
						filepath.Join(
							tempDir,
							filepath.FromSlash(link.from),
						),
					),
					fs.ModePerm,
				)
				if err != nil {
					panic(err)
				}

				if link.to != "" {
					err = os.Symlink(link.to, filepath.Join(tempDir, filepath.FromSlash(link.from)))
					if err != nil {
						panic(err)
					}
				}
			}

			resolved, err := ResolvePath(
				osfsLite{tempDir},
				filepath.FromSlash(tc.from),
				false,
			)

			if tc.resolved != filepath.ToSlash(resolved) {
				t.Errorf(
					"resolved not equal: expected(%q) != actual(%q) (filepath.ToSlash is applied to print this result)",
					tc.resolved, filepath.ToSlash(resolved),
				)
			}
			if !errors.Is(err, tc.err) {
				t.Errorf("err does not satisfy errors.Is(err, %v), but is %v", tc.err, err)
			}
		})
	}
}

func TestResolveSymlink(t *testing.T) {
	tempDir := t.TempDir()

	err := os.Mkdir(filepath.Join(tempDir, "foo"), fs.ModePerm)
	if err != nil {
		panic(err)
	}

	for i := range 5 {
		err = os.Symlink(
			strconv.FormatInt(int64(i+1), 10),
			filepath.Join(tempDir, "foo", strconv.FormatInt(int64(i), 10)),
		)
		if err != nil {
			panic(err)
		}
	}
	f, err := os.Create(filepath.Join(tempDir, "foo", "5"))
	if err != nil {
		panic(err)
	}
	_ = f.Close()

	_, num, _ := ResolveSymlink(
		osfsLite{tempDir},
		filepath.Join("foo", "0"),
		20,
	)

	if num != 5 {
		t.Errorf("not euql: expected(%d) != actual(%d)", 5, num)
	}
}

func TestResolveSymlink_non_link(t *testing.T) {
	tempDir := t.TempDir()
	err := os.Mkdir(filepath.Join(tempDir, "a"), fs.ModePerm)
	if err != nil {
		panic(err)
	}

	_, _, err = ResolveSymlink(osfsLite{tempDir}, "a", 10)
	if errors.Is(err, fs.ErrInvalid) {
		t.Errorf("expected fs.ErrInvalid, but is %v", err)
	}
}

func TestResolveSymlink_empty_path(t *testing.T) {
	tempDir := t.TempDir()
	resolved, num, err := ResolveSymlink(osfsLite{tempDir}, "", 30)
	if resolved != "" || num != 0 || err != nil {
		t.Errorf(
			"incorrect: expected \"\", 0 and nil error, but is %q, %d and %v",
			resolved, num, err,
		)
	}
}

func TestResolvePath_ErrorPaths(t *testing.T) {
	tempDir := t.TempDir()
	fsys := osfsLite{tempDir}

	t.Run("Lstat error on non-existent path", func(t *testing.T) {
		_, err := ResolvePath(fsys, "nonexistent/path", false)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Error("expected error for non-existent path")
		}
	})

	t.Run("ResolveSymlink error propagation", func(t *testing.T) {
		// Create a broken symlink
		if err := os.Symlink("nonexistent", filepath.Join(tempDir, "broken")); err != nil {
			t.Fatalf("failed to create broken symlink: %v", err)
		}

		_, err := ResolvePath(fsys, "broken", false)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Error("expected error for broken symlink")
		}
	})

	t.Run("symlink with absolute target path escapes", func(t *testing.T) {
		// Create a symlink pointing to absolute path
		if err := os.Symlink("/etc/passwd", filepath.Join(tempDir, "escape")); err != nil {
			t.Fatalf("failed to create escaping symlink: %v", err)
		}

		_, err := ResolvePath(fsys, "escape", false)
		if !errors.Is(err, ErrPathEscapes) {
			t.Error("expected error for escaping symlink")
		}
	})

	t.Run("symlink with .. target path escapes", func(t *testing.T) {
		// Create a symlink pointing to parent directory
		if err := os.Symlink("../secret", filepath.Join(tempDir, "dotdot")); err != nil {
			t.Fatalf("failed to create .. symlink: %v", err)
		}

		_, err := ResolvePath(fsys, "dotdot", false)
		if !errors.Is(err, ErrPathEscapes) {
			t.Error("expected error for .. symlink")
		}
	})

	t.Run("ResolvePath skipLastElement true", func(t *testing.T) {
		// Create a regular file
		if err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		resolved, err := ResolvePath(fsys, "file.txt", true)
		if err != nil {
			t.Fatalf("ResolvePath failed: %v", err)
		}

		// With skipLastElement=true on a filename with no separator, it returns the original name
		if resolved != "file.txt" {
			t.Errorf("expected resolved path 'file.txt', got %q", resolved)
		}

		// Test with a path that has a separator
		if err := os.MkdirAll(filepath.Join(tempDir, "subdir"), 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, "subdir", "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file in subdir: %v", err)
		}

		resolved, err = ResolvePath(fsys, filepath.FromSlash("subdir/file.txt"), true)
		if err != nil {
			t.Fatalf("ResolvePath failed: %v", err)
		}

		expected := filepath.FromSlash("subdir/file.txt")
		if resolved != expected {
			t.Errorf("expected resolved path %q, got %q", expected, resolved)
		}
	})
}

func TestResolveSymlink_targeting_each_other_ELOOP(t *testing.T) {
	tempDir := t.TempDir()

	var err error
	err = os.MkdirAll(filepath.Join(tempDir, "root"), fs.ModePerm)
	if err != nil {
		panic(err)
	}

	err = os.Symlink("./a", filepath.Join(tempDir, "root", "b"))
	if err != nil {
		panic(err)
	}
	err = os.Symlink("./b", filepath.Join(tempDir, "root", "a"))
	if err != nil {
		panic(err)
	}

	fsys := osfsLite{filepath.Join(tempDir, "root")}
	resolved, num, err := ResolveSymlink(fsys, "a", 30)
	if resolved != "" || num != 30 || !errors.Is(err, syscall.ELOOP) {
		t.Errorf(
			"incorrect: expected \"\", %d and error that satisfies erorrs.Is(err, syscall.ELOOP), but is %q, %d and %v",
			maxSymlinkResolutionCount, resolved, num, err,
		)
	}
}
