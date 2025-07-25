package testhelper

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

// Type checking with real implementation
var _ = func() {
	var osfsLite *osfslite.OsfsLite
	var t T
	var testingT *testing.T
	var extendedT ExtendedT

	OpenFile(t, osfsLite, "test.txt", os.O_RDONLY, 0o644, func(t ExtendedT, file *os.File) {})
	Open(testingT, osfsLite, "test.txt", func(t ExtendedT, file *os.File) {})
	Create(testingT, osfsLite, "test.txt", func(t ExtendedT, file *os.File) {})
	AssertContent(extendedT, osfsLite, "test.txt", []byte("content"))
	AssertAccessible(extendedT, osfsLite, "test.txt")
}

func TestOpenFile(t *testing.T) {
	tempDir := t.TempDir()
	testContent := []byte("test content")

	mt := new(mockT)
	mt.tempDir = tempDir

	fsys := &mockFsys{
		openFileFunc: func(name string, flag int, perm fs.FileMode) (*mockFile, error) {
			if name == "test.txt" && flag == os.O_RDONLY && perm == 0o644 {
				return &mockFile{
					content: testContent,
					closed:  false,
				}, nil
			}
			return &mockFile{}, errors.New("unexpected parameters")
		},
	}

	called := false
	OpenFile(mt, fsys, "test.txt", os.O_RDONLY, 0o644, func(t ExtendedT, file *mockFile) {
		called = true
		if file.closed {
			t.Errorf("file should not be closed inside callback")
		}
		content, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("failed to read file: %v", err)
		}
		if !bytes.Equal(content, testContent) {
			t.Errorf("content mismatch: expected %q, got %q", testContent, content)
		}
	})

	if !called {
		t.Errorf("callback was not called")
	}

	// Test error case
	mt = new(mockT)
	fsys.openFileFunc = func(name string, flag int, perm fs.FileMode) (*mockFile, error) {
		return &mockFile{}, errors.New("open failed")
	}

	// Run in goroutine to handle Goexit
	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
		}()
		OpenFile(mt, fsys, "nonexistent.txt", os.O_RDONLY, 0o644, func(t ExtendedT, file *mockFile) {
			t.Errorf("callback should not be called on error")
		})
	}()
	<-done

	if !mt.fatalfCalled {
		t.Errorf("Fatalf should be called on open error")
	}

	expectedFormat := `
Op: OpenFile
Path: nonexistent.txt

failed: %v`
	if mt.fatalfFormat != expectedFormat {
		t.Errorf("error format mismatch: expected %q, got %q", expectedFormat, mt.fatalfFormat)
	}
}

func TestOpen(t *testing.T) {
	testContent := []byte("open test content")

	fsys := &mockFsys{
		openFunc: func(name string) (*mockFile, error) {
			if name == "test.txt" {
				return &mockFile{
					content: testContent,
					closed:  false,
				}, nil
			}
			return &mockFile{}, errors.New("file not found")
		},
	}

	called := false
	Open(t, fsys, "test.txt", func(t ExtendedT, file *mockFile) {
		called = true
		if file.closed {
			t.Errorf("file should not be closed inside callback")
		}
		content, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("failed to read file: %v", err)
		}
		if !bytes.Equal(content, testContent) {
			t.Errorf("content mismatch: expected %q, got %q", testContent, content)
		}
	})

	if !called {
		t.Errorf("callback was not called")
	}
}

func TestCreate(t *testing.T) {
	fsys := &mockFsys{
		createFunc: func(name string) (*mockFile, error) {
			if name == "new.txt" {
				return &mockFile{
					content: []byte{},
					closed:  false,
				}, nil
			}
			return &mockFile{}, errors.New("create failed")
		},
	}

	called := false
	Create(t, fsys, "new.txt", func(t ExtendedT, file *mockFile) {
		called = true
		if file.closed {
			t.Errorf("file should not be closed inside callback")
		}
		// Write something to the file
		n, err := file.Write([]byte("created content"))
		if err != nil {
			t.Errorf("failed to write: %v", err)
		}
		if n != 15 {
			t.Errorf("wrote %d bytes, expected 15", n)
		}
	})

	if !called {
		t.Errorf("callback was not called")
	}
}

func TestAssertContent_HappyPath(t *testing.T) {
	type testCase struct {
		name            string
		fileContent     []byte
		expectedContent []byte
	}

	cases := []testCase{
		{
			name:            "equal content",
			fileContent:     []byte("hello world"),
			expectedContent: []byte("hello world"),
		},
		{
			name:            "empty content",
			fileContent:     []byte{},
			expectedContent: []byte{},
		},
		{
			name:            "multiline content",
			fileContent:     []byte("line1\nline2\nline3"),
			expectedContent: []byte("line1\nline2\nline3"),
		},
		{
			name:            "special characters",
			fileContent:     []byte("hello\tworld\n\r"),
			expectedContent: []byte("hello\tworld\n\r"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mt := new(mockT)
			ext := wrapT(T(mt))

			fsys := &mockFsys{
				openFunc: func(name string) (*mockFile, error) {
					return &mockFile{
						content: tc.fileContent,
					}, nil
				},
			}

			AssertContent(ext, fsys, "test.txt", tc.expectedContent)

			if mt.fatalCalled || mt.fatalfCalled {
				t.Errorf("unexpected fatal call: fatalCalled=%v, fatalfCalled=%v", mt.fatalCalled, mt.fatalfCalled)
				if mt.fatalCalled {
					t.Errorf("fatal args: %v", mt.fatalArgs)
				}
				if mt.fatalfCalled {
					t.Errorf("fatalf format: %s, args: %v", mt.fatalfFormat, mt.fatalfArgs)
				}
			}
		})
	}
}

func TestAssertContent_Errors(t *testing.T) {
	type testCase struct {
		name            string
		fileContent     []byte
		expectedContent []byte
		openError       error
		readError       error
	}

	cases := []testCase{
		{
			name:            "different content",
			fileContent:     []byte("hello world"),
			expectedContent: []byte("goodbye world"),
		},
		{
			name:            "prefix difference",
			fileContent:     []byte("aello world"),
			expectedContent: []byte("hello world"),
		},
		{
			name:            "suffix difference",
			fileContent:     []byte("hello worla"),
			expectedContent: []byte("hello world"),
		},
		{
			name:            "middle difference",
			fileContent:     []byte("hello xorld"),
			expectedContent: []byte("hello world"),
		},
		{
			name:            "longer actual",
			fileContent:     []byte("hello world!!!"),
			expectedContent: []byte("hello world"),
		},
		{
			name:            "longer expected",
			fileContent:     []byte("hello"),
			expectedContent: []byte("hello world"),
		},
		{
			name:      "open error",
			openError: errors.New("file not found"),
		},
		{
			name:            "read error",
			fileContent:     []byte("partial content"),
			expectedContent: []byte("partial content"),
			readError:       errors.New("read failed"),
		},
		{
			name:      "open error with specific message",
			openError: errors.New("permission denied"),
		},
		{
			name:            "read error with partial data",
			fileContent:     []byte("some data"),
			expectedContent: []byte("some data"),
			readError:       errors.New("unexpected EOF"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mt := new(mockT)
			ext := wrapT(T(mt))

			fsys := &mockFsys{
				openFunc: func(name string) (*mockFile, error) {
					if tc.openError != nil {
						return &mockFile{}, tc.openError
					}
					return &mockFile{
						content:   tc.fileContent,
						readError: tc.readError,
					}, nil
				},
			}

			// Run in goroutine to handle Goexit
			done := make(chan bool)
			go func() {
				defer func() {
					done <- true
				}()
				AssertContent(ext, fsys, "test.txt", tc.expectedContent)
			}()
			<-done

			if !mt.fatalCalled && !mt.fatalfCalled {
				t.Errorf("expected fatal to be called, but it wasn't")
			}
		})
	}
}

func TestAssertContent_ErrorMessageFormat(t *testing.T) {
	type testCase struct {
		name                 string
		fileContent          []byte
		expectedContent      []byte
		expectedErrorMessage string
		expectedPrefix       string
	}

	cases := []testCase{
		{
			name:                 "complete difference",
			fileContent:          []byte("hello world"),
			expectedContent:      []byte("goodbye world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "goodbye world"
actual: "hello world"

`,
		},
		{
			name:                 "prefix difference",
			fileContent:          []byte("aello world"),
			expectedContent:      []byte("hello world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "hello world"
actual: "aello world"

`,
		},
		{
			name:                 "suffix difference",
			fileContent:          []byte("hello worla"),
			expectedContent:      []byte("hello world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "hello world"
actual: "hello worla"

`,
		},
		{
			name:                 "middle difference with common prefix and suffix",
			fileContent:          []byte("hello xorld"),
			expectedContent:      []byte("hello world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "hello world"
actual: "hello xorld"

`,
		},
		{
			name:                 "longer actual with common prefix",
			fileContent:          []byte("hello world!!!"),
			expectedContent:      []byte("hello world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "hello world"
actual: "hello world!!!"

`,
		},
		{
			name:                 "longer expected",
			fileContent:          []byte("hello"),
			expectedContent:      []byte("hello world"),
			expectedErrorMessage: "not equal",
			expectedPrefix: `
Path: test.txt
expected: "hello world"
actual: "hello"

`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mt := new(mockT)
			ext := wrapT(T(mt))

			fsys := &mockFsys{
				openFunc: func(name string) (*mockFile, error) {
					return &mockFile{
						content: tc.fileContent,
					}, nil
				},
			}

			// Run in goroutine to handle Goexit
			done := make(chan bool)
			go func() {
				defer func() {
					done <- true
				}()
				AssertContent(ext, fsys, "test.txt", tc.expectedContent)
			}()
			<-done

			if !mt.fatalCalled {
				t.Errorf("expected Fatal to be called for content mismatch, but it wasn't")
				return
			}

			// Content mismatch should use Fatal, not Fatalf
			if len(mt.fatalArgs) != 1 {
				t.Errorf("Fatal should have 1 arg (slice), got %d: %v", len(mt.fatalArgs), mt.fatalArgs)
				return
			}
			argsSlice, ok := mt.fatalArgs[0].([]any)
			if !ok {
				t.Errorf("Fatal arg should be a slice, got %T: %v", mt.fatalArgs[0], mt.fatalArgs[0])
				return
			}
			if len(argsSlice) != 2 {
				t.Errorf("Fatal args slice should have 2 elements (prefix + message), got %d: %v", len(argsSlice), argsSlice)
				return
			}

			// Check the message prefix and content
			prefix := argsSlice[0].(string)
			message := argsSlice[1].(string)

			// Use exact matching for both prefix and error message
			if prefix != tc.expectedPrefix {
				t.Errorf("not equal:\nexpected: %#v\nactual: %#v\n", tc.expectedPrefix, prefix)
			}

			if message != tc.expectedErrorMessage {
				t.Errorf("not equal: expected(%q) != actual(%q)", tc.expectedErrorMessage, message)
			}
		})
	}
}

func TestAssertAccessible(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		mt := new(mockT)
		ext := wrapT(T(mt))

		fsys := &mockFsys{
			statFunc: func(name string) (fs.FileInfo, error) {
				return &mockFileInfo{
					name: "test.txt",
					size: 10,
					mode: 0o644,
				}, nil
			},
		}

		AssertAccessible(ext, fsys, "test.txt")

		if mt.fatalCalled || mt.fatalfCalled {
			t.Errorf("unexpected fatal call: fatalCalled=%v, fatalfCalled=%v", mt.fatalCalled, mt.fatalfCalled)
		}
	})

	t.Run("file not exists", func(t *testing.T) {
		mt := new(mockT)
		ext := wrapT(T(mt))

		fsys := &mockFsys{
			statFunc: func(name string) (fs.FileInfo, error) {
				return nil, fs.ErrNotExist
			},
		}

		// Run in goroutine to handle Goexit
		done := make(chan bool)
		go func() {
			defer func() {
				done <- true
			}()
			AssertAccessible(ext, fsys, "missing.txt")
		}()
		<-done

		if !mt.fatalfCalled {
			t.Errorf("expected Fatalf to be called for missing file")
		}

		expectedFormat := `
Op: Stat
Path: missing.txt

failed: %v`
		if mt.fatalfFormat != expectedFormat {
			t.Errorf("error format mismatch: expected %q, got %q", expectedFormat, mt.fatalfFormat)
		}
	})
}

// mockFsys implements a filesystem for testing
type mockFsys struct {
	openFileFunc func(name string, flag int, perm fs.FileMode) (*mockFile, error)
	openFunc     func(name string) (*mockFile, error)
	createFunc   func(name string) (*mockFile, error)
	statFunc     func(name string) (fs.FileInfo, error)
}

func (m *mockFsys) OpenFile(name string, flag int, perm fs.FileMode) (*mockFile, error) {
	if m.openFileFunc != nil {
		return m.openFileFunc(name, flag, perm)
	}
	return &mockFile{}, errors.New("not implemented")
}

func (m *mockFsys) Open(name string) (*mockFile, error) {
	if m.openFunc != nil {
		return m.openFunc(name)
	}
	return &mockFile{}, errors.New("not implemented")
}

func (m *mockFsys) Create(name string) (*mockFile, error) {
	if m.createFunc != nil {
		return m.createFunc(name)
	}
	return &mockFile{}, errors.New("not implemented")
}

func (m *mockFsys) Stat(name string) (fs.FileInfo, error) {
	if m.statFunc != nil {
		return m.statFunc(name)
	}
	return nil, errors.New("not implemented")
}

// mockFile implements a file for testing
type mockFile struct {
	content   []byte
	readPos   int
	closed    bool
	readError error
}

func (m *mockFile) Read(p []byte) (int, error) {
	if m.closed {
		return 0, errors.New("file is closed")
	}
	if m.readError != nil {
		// Return partial read with error
		if m.readPos < len(m.content) {
			n := copy(p, m.content[m.readPos:])
			m.readPos += n
			return n, m.readError
		}
		return 0, m.readError
	}
	if m.readPos >= len(m.content) {
		return 0, io.EOF
	}
	n := copy(p, m.content[m.readPos:])
	m.readPos += n
	if m.readPos >= len(m.content) {
		return n, io.EOF
	}
	return n, nil
}

func (m *mockFile) Write(p []byte) (int, error) {
	if m.closed {
		return 0, errors.New("file is closed")
	}
	m.content = append(m.content, p...)
	return len(p), nil
}

func (m *mockFile) Close() error {
	if m.closed {
		return errors.New("already closed")
	}
	m.closed = true
	return nil
}

// mockFileInfo implements fs.FileInfo for testing
type mockFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }
