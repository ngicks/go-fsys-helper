package fsutil

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
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
	opener func(fsys osfslite.OsfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
	tc openFileRandomTestCase,
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := *osfslite.New(tempDir)

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
	opener func(fsys osfslite.OsfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := *osfslite.New(tempDir)

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
	opener func(fsys osfslite.OsfsLite, dir string, pattern string, perm fs.FileMode) (*os.File, error),
) {
	t.Helper()

	tempDir := t.TempDir()
	fsys := *osfslite.New(tempDir)

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
		t.Errorf("expected 30 unique file names, got %d", len(names))
	}
}

func TestOpenRandom_ErrorPaths(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("OpenFile permission denied", func(t *testing.T) {
		// Create a directory
		roDir := filepath.Join(tempDir, "readonly")
		if err := os.Mkdir(roDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		// Use mock filesystem that rejects OpenFile
		mockFs := &mockErrorFs{
			OsfsLite:          *osfslite.New(roDir),
			openFileError:     fs.ErrPermission,
			openFileErrorPath: ".tmp",
		}
		_, err := OpenFileRandom(mockFs, ".", "*.tmp", 0o644)
		if err == nil {
			t.Error("expected error when creating file with mock permission denied")
		}
	})

	t.Run("MkdirRandom permission denied", func(t *testing.T) {
		// Create a directory
		roDir := filepath.Join(tempDir, "readonly2")
		if err := os.Mkdir(roDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		// Use mock filesystem that rejects Mkdir
		mockFs := &mockErrorFs{
			OsfsLite:       *osfslite.New(roDir),
			mkdirError:     fs.ErrPermission,
			mkdirErrorPath: ".tmp",
		}
		_, err := MkdirRandom[*mockErrorFs](mockFs, ".", "*.tmp", 0o755)
		if err == nil {
			t.Error("expected error when creating directory with mock permission denied")
		}
	})

	t.Run("openRandom with max retry exceeded", func(t *testing.T) {
		// Create a mock filesystem that always returns ErrExist for file creation
		mockFs := &mockFsAlwaysExists{}

		// Test OpenFileRandom with max retry
		_, err := OpenFileRandom(mockFs, ".", "*.tmp", 0o644)
		if !errors.Is(err, ErrMaxRetry) {
			t.Errorf("expected ErrMaxRetry, got: %v", err)
		}
	})

	t.Run("mkdirRandom with max retry exceeded", func(t *testing.T) {
		// Create a mock filesystem that always returns ErrExist for directory creation
		mockFs := &mockFsAlwaysExists{}

		// Test MkdirRandom with max retry
		_, err := MkdirRandom(mockFs, ".", "*.tmp", 0o755)
		if !errors.Is(err, ErrMaxRetry) {
			t.Errorf("expected ErrMaxRetry, got: %v", err)
		}
	})
}

// mockFsAlwaysExists is a mock filesystem that always returns fs.ErrExist for file/directory creation,
// used to test max retry behavior that results in ErrMaxRetry
type mockFsAlwaysExists struct{}

func (m *mockFsAlwaysExists) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrExist}
}

func (m *mockFsAlwaysExists) Mkdir(name string, perm fs.FileMode) error {
	return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
}
