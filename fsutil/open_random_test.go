package fsutil

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type openFileRandomTestCase struct {
	name    string
	dir     string
	pattern string
}

var openRandomCases = []openFileRandomTestCase{
	{
		name: "basic file creation",
		dir:  ".",
	},
	{
		name:    "no wildcard",
		dir:     ".",
		pattern: "fixed.txt",
	},
	{
		name:    "subdirectory",
		dir:     "subdir",
		pattern: "prefix*suffix",
	},
	{
		name:    "empty dir",
		dir:     "",
		pattern: "empty*.log",
	},
}

func TestOpenFileRandom(t *testing.T) {
	for _, tc := range openRandomCases {
		t.Run(tc.name, func(t *testing.T) { testOpenRandom(t, OpenFileRandom, tc) })
	}
}

func TestMkdirRandom(t *testing.T) {
	for _, tc := range openRandomCases {
		t.Run(tc.name, func(t *testing.T) { testOpenRandom(t, MkdirRandom, tc) })
	}
}

func testOpenRandom(
	t *testing.T,
	opener func(fsys osfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
	tc openFileRandomTestCase,
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := osfsLite{tempDir}

	if tc.dir != "" && tc.dir != "." {
		err := os.MkdirAll(filepath.Join(tempDir, filepath.FromSlash(tc.dir)), fs.ModePerm)
		if err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	file, err := opener(
		fsys,
		filepath.FromSlash(tc.dir),
		filepath.FromSlash(tc.pattern),
		fs.ModePerm,
	)
	if err != nil {
		t.Fatalf("OpenFileRandom failed: %v", err)
	}
	defer file.Close()

	name := file.Name()
	if !strings.HasPrefix(filepath.Base(name), strings.Split(tc.pattern, "*")[0]) {
		t.Errorf("file name %q doesn't match pattern prefix %q", name, tc.pattern)
	}

	if strings.Contains(tc.pattern, "*") {
		suffix := strings.Split(tc.pattern, "*")[1]
		if suffix != "" && !strings.HasSuffix(filepath.Base(name), suffix) {
			t.Errorf("file name %q doesn't match pattern suffix %q", name, suffix)
		}
	}

	s, err := file.Stat()
	if err != nil {
		panic(err)
	}

	if s.Mode().IsRegular() {
		_, err = file.Write([]byte("test content"))
		if err != nil {
			t.Errorf("failed to write to file: %v", err)
		}
	}
}

func TestOpenFileRandom_BadPattern(t *testing.T) {
	testOpenRandomBadPattern(t, OpenFileRandom)
}

func TestMkdirRandom_BadPattern(t *testing.T) {
	testOpenRandomBadPattern(t, MkdirRandom)
}

func testOpenRandomBadPattern(
	t *testing.T,
	opener func(fsys osfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := osfsLite{tempDir}

	_, err := opener(
		fsys,
		filepath.FromSlash("."),
		filepath.FromSlash("bad/pattern*.txt"),
		0o644,
	)
	if err == nil {
		t.Error("expected error for pattern with path separator")
	}
	if !strings.Contains(err.Error(), "bad pattern") {
		t.Errorf("expected 'bad pattern' error, got: %v", err)
	}
}

func TestOpenFileRandom_MultipleFiles(t *testing.T) {
	testOpenRandomMultipleFiles(t, OpenFileRandom)
}

func TestMkdirRandom_MultipleFiles(t *testing.T) {
	testOpenRandomMultipleFiles(t, MkdirRandom)
}

func testOpenRandomMultipleFiles(
	t *testing.T,
	opener func(fsys osfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := osfsLite{tempDir}

	var files []*os.File
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()

	names := make(map[string]bool)
	for i := range 30 {
		file, err := opener(fsys, ".", "test*.txt", 0o644)
		if err != nil {
			t.Fatalf("OpenFileRandom failed on iteration %d: %v", i, err)
		}
		files = append(files, file)

		name := filepath.Base(file.Name())
		if names[name] {
			t.Errorf("duplicate file name generated: %s", name)
		}
		names[name] = true

		after, ok := strings.CutPrefix(name, "test")
		if !ok {
			t.Errorf("invalid name %q", name)
		}
		before, ok := strings.CutSuffix(after, ".txt")
		if !ok {
			t.Errorf("invalid name %q", name)
		}
		if len(before) != len(strconv.FormatUint(uint64(math.MaxUint32), 10)) {
			t.Errorf("invalid name %q", name)
		}
		if len(strings.TrimLeftFunc(before, func(r rune) bool { return '0' <= r && r <= '9' })) != 0 {
			t.Errorf("invalid name %q", name)
		}
	}

	if len(names) != 30 {
		t.Errorf("expected 10 unique file names, got %d", len(names))
	}
}
