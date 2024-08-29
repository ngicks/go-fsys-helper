package fsutil

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// ignoreHiddenFile masks fsys so that names prefixed with '.' are hidden from a returned fs.FS.
func ignoreHiddenFile(fsys fs.FS) fs.FS {
	return afero.NewIOFS(afero.NewRegexpFs(afero.FromIOFS{FS: fsys}, regexp.MustCompile(`(?:^|\/)[^.].*`)))
}

type safeWriteTestCaseBase struct {
	name        string
	Description string
	opts        []SafeWriteOptionOption
	writeArgs   safeWriteArgs
	writeFsArgs safeWriteFsArgs
}

type safeWriteArgs struct {
	path          string
	perm          fs.FileMode
	r             io.Reader
	postProcesses []SafeWritePostProcess
}

type safeWriteFsArgs struct {
	dir           string
	perm          fs.FileMode
	src           fs.FS
	postProcesses []SafeWritePostProcess
}

func TestSafeWrite(t *testing.T) {
	type testCase struct {
		safeWriteTestCaseBase
		assertBeforeRename []assertBefore
		assertResult       []assertAfter
	}

	// cases with no error.
	for _, tc := range []testCase{
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "SafeWrite, default option",
				opts: []SafeWriteOptionOption{},
				writeArgs: safeWriteArgs{
					path: "foo/bar/baz",
					perm: fs.ModePerm,
					r:    bytes.NewBufferString("baz"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`baz-\d{10}.tmp$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("baz")}}),
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "SafeWriteFs, default option",
				opts: []SafeWriteOptionOption{},
				writeFsArgs: safeWriteFsArgs{
					dir:  "foo/bar/baz",
					perm: fs.ModePerm,
					src:  os.DirFS("testdata/fs6"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathContains(`^/?foo/bar/baz-\d{10}.tmp\/`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("foo/bar/baz", ignoreHiddenFile(os.DirFS("testdata/fs6"))),
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name:        "SafeWrite, tmp dir",
				Description: "if tmp dir is non empty, suffix default fallback is disabled.",
				opts:        []SafeWriteOptionOption{WithTmpDir("/tmp")},
				writeArgs: safeWriteArgs{
					path: "foo/bar/baz",
					perm: fs.ModePerm,
					r:    bytes.NewBufferString("addawd90i2;p"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`^/?tmp/baz-\d{10}$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("addawd90i2;p")}}),
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "SafeWrite, tmp dir with explicit suffix",
				opts: []SafeWriteOptionOption{WithTmpDir("/tmp"), must(WithPrefixSuffix("pref-", ".tmp"))},
				writeArgs: safeWriteArgs{
					path: "foo/bar/baz",
					perm: fs.ModePerm,
					r:    bytes.NewBufferString("addawd90i2;p"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`^/?tmp/pref-baz-\d{10}.tmp$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("addawd90i2;p")}}),
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "SafeWriteFs, tmp dir, explicit suffix, without rand suffix",
				opts: []SafeWriteOptionOption{
					WithTmpDir("tmptmp"),
					must(WithPrefixSuffix("temp-pref-", ".suf")),
					must(WithRandomPattern("")),
				},
				writeFsArgs: safeWriteFsArgs{
					dir:  "nah/nay",
					perm: fs.ModePerm,
					src:  os.DirFS("testdata/fs1"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(2), // 2 files
				assertPathContains(`^/?tmptmp/temp-pref-nay.suf\/`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("nah/nay", os.DirFS("testdata/fs1")),
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name:        "SafeWriteFs, ForcePerm",
				Description: "ForcePerm for SafeWriteFs only invoke chmod on targeted dir, contents under the dir are unaffected",
				opts:        []SafeWriteOptionOption{WithForcePerm(true), WithTmpDir("tmp"), must(WithRandomPattern(""))},
				writeFsArgs: safeWriteFsArgs{
					dir:  "foo",
					perm: 0o410,
					src:  os.DirFS("testdata/fs4"),
				},
			},
			assertBeforeRename: []assertBefore{
				assertLen(2), // 2 files
				assertPathContains(`^/?tmp\/foo`),
				assertPerm("/tmp/foo", fs.ModeDir|0710),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("foo", os.DirFS("testdata/fs4")),
				assertModeAfter("foo", fs.ModeDir|0710),
			},
		},
	} {

		for _, preparer := range [](func() (string, afero.Fs, func())){
			prepareMemFs,
			prepareTmpFs,
			prepareSubMemFs,
			prepareDeeplyNestedBasePathFs,
		} {
			name, baseFsys, clean := preparer()
			defer clean()
			tc := tc
			t.Run(tc.name+", "+name, func(t *testing.T) {
				var err error

				opt := NewSafeWriteOption(tc.opts...)

				assertCalled := false
				seenPathsBefore, seenPathsAfter := []string{}, []string{}
				assertBeforeRename := func(fsys afero.Fs, _, _ string, file afero.File) error {
					assertCalled = true
					seenPathsBefore := collectPath(fsys)
					for _, assert := range tc.assertBeforeRename {
						assert(t, fsys, seenPathsBefore)
					}
					return nil
				}

				if tc.safeWriteTestCaseBase.writeArgs.path != "" {
					err = opt.SafeWrite(
						baseFsys,
						tc.writeArgs.path,
						tc.writeArgs.perm,
						tc.writeArgs.r,
						append(tc.writeArgs.postProcesses, assertBeforeRename)...,
					)
				} else {
					err = opt.SafeWriteFs(
						baseFsys,
						tc.writeFsArgs.dir,
						tc.writeFsArgs.perm,
						ignoreHiddenFile(tc.writeFsArgs.src),
						append(tc.writeFsArgs.postProcesses, assertBeforeRename)...,
					)
				}

				assert.NilError(t, err)
				if tc.assertBeforeRename != nil && !assertCalled {
					t.Fatalf("assertBeforeRename is defined but was not called")
				}

				err = fs.WalkDir(afero.NewIOFS(baseFsys), "/", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if path == "." || d.IsDir() {
						return nil
					}
					seenPathsAfter = append(seenPathsAfter, path)
					return nil
				})
				assert.NilError(t, err)

				for _, assert := range tc.assertResult {
					assert(t, baseFsys, err)
				}

				for _, path := range seenPathsBefore {
					_ = baseFsys.MkdirAll(filepath.Dir(path), fs.ModePerm)
					f, err := baseFsys.Create(path)
					assert.NilError(t, err)
					assert.NilError(t, f.Close())
				}

				err = opt.CleanTmp(baseFsys)

				assert.NilError(t, err)
				for _, path := range seenPathsBefore {
					_, err := baseFsys.Open(path)
					assert.Assert(t, os.IsNotExist(err))
				}
				for _, path := range seenPathsAfter {
					_, err := baseFsys.Open(path)
					assert.NilError(t, err)
				}
			})
		}
	}
}

func prepareTmpFs() (name string, fsys afero.Fs, clean func()) {
	tmpDir, err := os.MkdirTemp("", "fsutil-test-SafeWrite-*")
	if err != nil {
		panic(err)
	}

	baseFsys := afero.NewBasePathFs(afero.NewOsFs(), tmpDir)

	var b atomic.Bool
	return (*afero.BasePathFs)(nil).Name(), baseFsys, func() {
		if b.CompareAndSwap(false, true) {
			_ = os.RemoveAll(tmpDir)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func collectPath(fsys afero.Fs) []string {
	var paths []string
	err := fs.WalkDir(afero.NewIOFS(fsys), "/", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || d.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return paths
}

type assertBefore func(t *testing.T, fsys afero.Fs, seenPaths []string)

func assertLen(l int) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		assert.Assert(t, cmp.Len(seenPaths, l))
	}
}

func assertPathContains(pat string) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		reg := regexp.MustCompile(pat)
		for _, p := range seenPaths {
			if reg.MatchString(p) {
				return
			}
		}
		t.Fatalf("all path does not matched to pattern %s, paths = %#v", pat, seenPaths)
	}
}

func assertPathPattern(pat string) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		for _, p := range seenPaths {
			assert.Assert(t, cmp.Regexp(pat, p), "assertPathPattern")
		}
	}
}

func assertPerm(path string, mode fs.FileMode) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		s, err := fsys.Stat(path)
		assert.NilError(t, err, "assertPerm")
		assert.Assert(t, cmp.Equal(s.Mode(), mode), "assertPerm")
	}
}

type assertAfter func(t *testing.T, fsys afero.Fs, err error)

func assertNilErr() assertAfter {
	return func(t *testing.T, fsys afero.Fs, err error) {
		t.Helper()
		assert.NilError(t, err, "assertNilErr")
	}
}

type namedContent struct {
	path    string
	content *bytes.Buffer
}

func assertContents(namedContents []namedContent) assertAfter {
	return func(t *testing.T, fsys afero.Fs, err error) {
		t.Helper()
		for _, nc := range namedContents {
			f, err := fsys.Open(normalizePath(nc.path))
			assert.NilError(t, err, "assertContents")
			s, err := f.Stat()
			assert.NilError(t, err, "assertContents")
			same, err := sameReader(f, nc.content, s.Size(), int64(nc.content.Len()))
			assert.NilError(t, err, "assertContents")
			assert.Assert(t, same, "assertContents")
		}
	}
}

func assertFsUnder(base string, fsys fs.FS) assertAfter {
	return func(t *testing.T, fsys_ afero.Fs, _ error) {
		t.Helper()
		eq, err := Equal(fsys, afero.NewIOFS(afero.NewBasePathFs(fsys_, base)))
		assert.NilError(t, err, "assertFsUnder")
		assert.Assert(t, eq.Equal(), "assertFsUnder: not equal: reports = %v", eq)
	}
}

func assertModeAfter(path string, mode fs.FileMode) assertAfter {
	return func(t *testing.T, fsys afero.Fs, _ error) {
		t.Helper()
		s, err := fsys.Stat(normalizePath(path))
		assert.NilError(t, err, "assertModeAfter")
		assert.Assert(t, cmp.Equal(s.Mode(), mode), "assertModeAfter")
	}
}

var (
	errExample = errors.New("example")
)

// TestSafeWrite_DisableOptions tests where normally hard to observe states.
func TestSafeWrite_DisableOptions(t *testing.T) {
	type testCase struct {
		safeWriteTestCaseBase
		skipFs           func(fsys afero.Fs, name string) bool
		err              error
		assertFsOps      func([]ObservableFsOp)
		assertTmpFileOps func([]ObservableFsFileOp)
	}

	writeArgs := safeWriteArgs{
		path: "foo",
		perm: fs.ModePerm,
		r:    bytes.NewBufferString("foobarbaz"),
	}

	for _, tc := range []testCase{
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "normal cases",
			},
			assertTmpFileOps: func(offo []ObservableFsFileOp) {
				assertContainsFileOp(t, offo, ObservableFsFileOpNameSync)
				assertContainsFileOp(t, offo, ObservableFsFileOpNameClose)
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "disable sync",
				opts: []SafeWriteOptionOption{WithDisableSync(true)},
			},
			assertTmpFileOps: func(offo []ObservableFsFileOp) {
				assertNotContainsFileOp(t, offo, ObservableFsFileOpNameSync)
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "disabling mkdir causes ENOENT",
				opts: []SafeWriteOptionOption{WithDisableMkdir(true)},
			},
			err: fs.ErrNotExist,
			// Skimming through afero source code reveals MemMapFs does not checks parent directory existence for file creation.
			skipFs: func(fsys afero.Fs, name string) bool { return strings.Contains(name, (*afero.MemMapFs)(nil).Name()) },
			assertFsOps: func(ofo []ObservableFsOp) {
				assertContainsFsOp(t, ofo, ObservableFsOpNameOpenFile)
				assert.Assert(t, cmp.Len(ofo, 1))
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "returning an error in postProcess exists early",
				writeArgs: safeWriteArgs{
					postProcesses: []SafeWritePostProcess{func(fsys afero.Fs, _, _ string, file afero.File) error {
						return errExample
					}},
				},
			},
			err: errExample,
			assertFsOps: func(ofo []ObservableFsOp) {
				assertContainsFsOp(t, ofo, ObservableFsOpNameRemoveAll)
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "a matching error to ignoreMatchedErr leaves the tmp file in tact",
				opts: []SafeWriteOptionOption{WithIgnoreMatchedErr(func(err error) bool { return errors.Is(err, errExample) })},
				writeArgs: safeWriteArgs{
					postProcesses: []SafeWritePostProcess{func(fsys afero.Fs, _, _ string, file afero.File) error {
						return errExample
					}},
				},
			},
			err: errExample,
			assertFsOps: func(ofo []ObservableFsOp) {
				assertNotContainsFsOp(t, ofo, ObservableFsOpNameRemoveAll)
				assertNotContainsFsOp(t, ofo, ObservableFsOpNameRename)
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "a mismatching error to ignoreMatchedErr still removes the tmp file",
				opts: []SafeWriteOptionOption{WithIgnoreMatchedErr(func(err error) bool { return errors.Is(err, ErrBadInput) })},
				writeArgs: safeWriteArgs{
					postProcesses: []SafeWritePostProcess{func(fsys afero.Fs, _, _ string, file afero.File) error {
						return errExample
					}},
				},
			},
			err: errExample,
			assertFsOps: func(ofo []ObservableFsOp) {
				assertContainsFsOp(t, ofo, ObservableFsOpNameRemoveAll)
				assertNotContainsFsOp(t, ofo, ObservableFsOpNameRename)
			},
		},
		{
			safeWriteTestCaseBase: safeWriteTestCaseBase{
				name: "disabling remove on error leaves the failed tmp file in tact",
				opts: []SafeWriteOptionOption{WithDisableRemoveOnErr(true)},
				writeArgs: safeWriteArgs{
					postProcesses: []SafeWritePostProcess{func(fsys afero.Fs, _, _ string, file afero.File) error {
						return errExample
					}},
				},
			},
			err: errExample,
			assertFsOps: func(ofo []ObservableFsOp) {
				assertNotContainsFsOp(t, ofo, ObservableFsOpNameRemoveAll)
				assertNotContainsFsOp(t, ofo, ObservableFsOpNameRename)
			},
		},
	} {
		for _, preparer := range [](func() (string, afero.Fs, func())){
			// prepareMemFs,
			// prepareTmpFs,
			// prepareSubMemFs,
			prepareDeeplyNestedBasePathFs,
		} {
			name, fsys, clean := preparer()
			defer clean()
			t.Run(tc.name+", "+name, func(t *testing.T) {
				if tc.skipFs != nil && tc.skipFs(fsys, name) {
					t.Skip()
				}
				// MemMapFs behaves differently from real fs.
				fsys := NewObservableFs(fsys)
				// for easier testing, disable randomness of name.
				opt := NewSafeWriteOption(append([]SafeWriteOptionOption{
					WithTmpDir("tmp"),
					must(WithRandomPattern("")),
				}, tc.opts...)...)

				args := mergeSafeWriteArgs(writeArgs, tc.writeArgs)
				err := opt.SafeWrite(fsys, args.path, args.perm, args.r, args.postProcesses...)
				assert.Assert(t, cmp.ErrorIs(err, tc.err))

				obs := fsys.Observer()

				if tc.assertFsOps != nil {
					tc.assertFsOps(obs.FsOp())
				}
				if tc.assertTmpFileOps != nil {
					tc.assertTmpFileOps(obs.FileOp(normalizePath(filepath.Join("tmp", args.path))))
				}
			})
		}
	}
}

func prepareMemFs() (string, afero.Fs, func()) {
	return (*afero.MemMapFs)(nil).Name(), afero.NewMemMapFs(), func() {}
}

func prepareSubMemFs() (string, afero.Fs, func()) {
	return (*afero.MemMapFs)(nil).Name() + "/foo", afero.NewBasePathFs(afero.NewMemMapFs(), "foo"), func() {}
}

func prepareDeeplyNestedBasePathFs() (string, afero.Fs, func()) {
	name, fsys, clean := prepareTmpFs()
	return name + "/foo/bar/baz",
		afero.NewBasePathFs(afero.NewBasePathFs(afero.NewBasePathFs(fsys, "foo"), "bar"), "baz"),
		clean
}

func mergeSafeWriteArgs(arg1, arg2 safeWriteArgs) safeWriteArgs {
	if arg2.path != "" {
		arg1.path = arg2.path
	}
	if arg2.perm != 0 {
		arg1.perm = arg2.perm
	}
	if arg2.r != nil {
		arg1.r = arg2.r
	}
	if arg2.postProcesses != nil {
		arg1.postProcesses = arg2.postProcesses
	}
	return arg1
}

func assertContainsFsOp(t *testing.T, ops []ObservableFsOp, op ObservableFsOpName) {
	t.Helper()
	assert.Assert(t, slices.ContainsFunc(ops, func(ofo ObservableFsOp) bool { return ofo.Op == op }))
}

func assertNotContainsFsOp(t *testing.T, ops []ObservableFsOp, op ObservableFsOpName) {
	t.Helper()
	assert.Assert(t, !slices.ContainsFunc(ops, func(ofo ObservableFsOp) bool { return ofo.Op == op }))
}

func assertContainsFileOp(t *testing.T, ops []ObservableFsFileOp, op ObservableFsFileOpName) {
	t.Helper()
	assert.Assert(t, slices.ContainsFunc(ops, func(offo ObservableFsFileOp) bool { return offo.Op == op }))
}

func assertNotContainsFileOp(t *testing.T, ops []ObservableFsFileOp, op ObservableFsFileOpName) {
	t.Helper()
	assert.Assert(t, !slices.ContainsFunc(ops, func(offo ObservableFsFileOp) bool { return offo.Op == op }))
}
