package synthfs_test

import (
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

func newEmpty(t *testing.T) *synthfs.Root {
	t.Helper()
	return synthfs.NewRoot("synth://test", nil)
}

func TestAddFile_Bytes(t *testing.T) {
	r := newEmpty(t)
	now := time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)
	v := synthfs.NewBytesView([]byte("hello"), 0o644, now)

	testhelper.NilErr(t, r.AddFile("greet.txt", v, nil))

	got, err := vroot.ReadFile(r, "greet.txt")
	testhelper.NilErr(t, err)
	if string(got) != "hello" {
		t.Fatalf("ReadFile: got %q, want %q", got, "hello")
	}

	info, err := r.Stat("greet.txt")
	testhelper.NilErr(t, err)
	if info.Size() != 5 {
		t.Errorf("size: got %d, want 5", info.Size())
	}
}

func TestAddFile_AutoCreatesParents(t *testing.T) {
	r := newEmpty(t)
	v := synthfs.NewBytesView([]byte("x"), 0o644, time.Now())

	testhelper.NilErr(t, r.AddFile("a/b/c/leaf.txt", v, nil))

	for _, p := range []string{"a", "a/b", "a/b/c"} {
		info, err := r.Stat(p)
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("scaffolded %q is not a directory", p)
		}
	}
}

func TestAddFS_MergesSources(t *testing.T) {
	r := newEmpty(t)
	srcA := fstest.MapFS{
		"a.txt": &fstest.MapFile{Data: []byte("AAA"), Mode: 0o644},
		"sub/x": &fstest.MapFile{Data: []byte("X"), Mode: 0o644},
	}
	srcB := fstest.MapFS{
		"b.txt": &fstest.MapFile{Data: []byte("BBB"), Mode: 0o644},
		"sub/y": &fstest.MapFile{Data: []byte("Y"), Mode: 0o644},
	}

	testhelper.NilErr(t, r.AddFS("pkg", srcA, nil))
	testhelper.NilErr(t, r.AddFS("pkg", srcB, nil))

	for _, p := range []string{"pkg/a.txt", "pkg/b.txt", "pkg/sub/x", "pkg/sub/y"} {
		if _, err := r.Stat(p); err != nil {
			t.Errorf("expected %q after merge: %v", p, err)
		}
	}
}

func TestAddFS_DefaultOverwritesLeaf(t *testing.T) {
	r := newEmpty(t)
	srcA := fstest.MapFS{"f.txt": &fstest.MapFile{Data: []byte("first"), Mode: 0o644}}
	srcB := fstest.MapFS{"f.txt": &fstest.MapFile{Data: []byte("second"), Mode: 0o644}}

	testhelper.NilErr(t, r.AddFS("pkg", srcA, nil))
	testhelper.NilErr(t, r.AddFS("pkg", srcB, nil))

	got, err := vroot.ReadFile(r, "pkg/f.txt")
	testhelper.NilErr(t, err)
	if string(got) != "second" {
		t.Errorf("expected 'second' to overwrite 'first', got %q", got)
	}
}

func TestAddFS_FailOnConflict(t *testing.T) {
	r := newEmpty(t)
	src := fstest.MapFS{"f.txt": &fstest.MapFile{Data: []byte("first"), Mode: 0o644}}

	testhelper.NilErr(t, r.AddFile("pkg/f.txt", synthfs.NewBytesView([]byte("preset"), 0o644, time.Now()), nil))

	err := r.AddFS("pkg", src, synthfs.FailOnConflict)
	if err == nil {
		t.Fatal("AddFS with FailOnConflict on existing leaf: want error, got nil")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("FailOnConflict error: got %v, want ErrExist", err)
	}
}

func TestAddFS_OverrideReplacesLeaf(t *testing.T) {
	r := newEmpty(t)
	testhelper.NilErr(t, r.AddFile("pkg/f.txt", synthfs.NewBytesView([]byte("preset"), 0o644, time.Now()), nil))

	src := fstest.MapFS{"f.txt": &fstest.MapFile{Data: []byte("override"), Mode: 0o644}}
	override := func(_, _ synthfs.AddEntry) (synthfs.AddDecision, error) {
		return synthfs.AddDecisionOverride, nil
	}
	testhelper.NilErr(t, r.AddFS("pkg", src, override))

	got, err := vroot.ReadFile(r, "pkg/f.txt")
	testhelper.NilErr(t, err)
	if string(got) != "override" {
		t.Errorf("after Override resolver: got %q, want %q", got, "override")
	}
}

func TestAddFS_MergeKeepPreservesExisting(t *testing.T) {
	r := newEmpty(t)
	testhelper.NilErr(t, r.AddFile("pkg/f.txt", synthfs.NewBytesView([]byte("preset"), 0o644, time.Now()), nil))

	src := fstest.MapFS{"f.txt": &fstest.MapFile{Data: []byte("override"), Mode: 0o644}}
	testhelper.NilErr(t, r.AddFS("pkg", src, synthfs.MergeKeep))

	got, err := vroot.ReadFile(r, "pkg/f.txt")
	testhelper.NilErr(t, err)
	if string(got) != "preset" {
		t.Errorf("after MergeKeep resolver: got %q, want %q", got, "preset")
	}
}

func TestAddFS_SkipPrunesSubtree(t *testing.T) {
	r := newEmpty(t)
	src := fstest.MapFS{
		"keep/a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"drop/b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
	}
	resolver := func(in, _ synthfs.AddEntry) (synthfs.AddDecision, error) {
		if in.Path() == "pkg/drop" {
			return synthfs.AddDecisionSkip, nil
		}
		return synthfs.AddDecisionOverride, nil
	}
	testhelper.NilErr(t, r.AddFS("pkg", src, resolver))

	if _, err := r.Stat("pkg/keep/a.txt"); err != nil {
		t.Errorf("kept entry missing: %v", err)
	}
	_, err := r.Stat("pkg/drop")
	if err == nil {
		t.Errorf("skipped subtree present")
	}
}

func TestFsFsConformance(t *testing.T) {
	r := newEmpty(t)
	src := fstest.MapFS{
		"a.txt":     &fstest.MapFile{Data: []byte("hello"), Mode: 0o644},
		"sub/b.txt": &fstest.MapFile{Data: []byte("world"), Mode: 0o644},
	}
	testhelper.NilErr(t, r.AddFS(".", src, nil))

	fsys := vroot.ToIoFsRoot(r)
	if err := fstest.TestFS(fsys, "a.txt", "sub/b.txt"); err != nil {
		t.Fatalf("fstest.TestFS: %v", err)
	}
}

func TestRangedView(t *testing.T) {
	inner := synthfs.NewBytesView([]byte("0123456789"), 0o644, time.Now())
	v, err := synthfs.NewRangedView(inner, 3, 4)
	testhelper.NilErr(t, err)

	r := newEmpty(t)
	testhelper.NilErr(t, r.AddFile("slice.bin", v, nil))

	f, err := r.Open("slice.bin")
	testhelper.NilErr(t, err)
	defer func() { _ = f.Close() }()

	got, err := io.ReadAll(f)
	testhelper.NilErr(t, err)
	if string(got) != "3456" {
		t.Errorf("ranged view content: got %q, want %q", got, "3456")
	}
}
