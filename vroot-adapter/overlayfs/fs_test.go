package overlayfs

import (
	"errors"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// fsysMaker hands out a fresh backing for one layer of a stack.
type fsysMaker func(t *testing.T, name string) vroot.Fs[vroot.File]

// eachBacking runs fn over every fsys implementation the overlay must work
// over: memfs, which is type-erased and in memory, and osfs, which has a
// concrete file type and a real directory under it.
func eachBacking(t *testing.T, fn func(t *testing.T, mk fsysMaker)) {
	t.Helper()
	for _, b := range []struct {
		name string
		mk   fsysMaker
	}{
		{"memfs", memfsMaker},
		{"osfs", func(t *testing.T, _ string) vroot.Fs[vroot.File] { return newOsfs(t, t.TempDir()) }},
	} {
		t.Run(b.name, func(t *testing.T) { fn(t, b.mk) })
	}
}

func memfsMaker(_ *testing.T, name string) vroot.Fs[vroot.File] { return memfs.New(name) }

// newOverlay stacks a canonical top over a canonical lower and a plain one and
// returns the merged view. The layers are closed by the data sources' own
// cleanup, so a test that does not exercise ownership need not close the Fs.
func newOverlay(t *testing.T, mk fsysMaker) (f *Fs, top, lower0, lower1 *DataSource) {
	t.Helper()
	top = canonicalSource(t, mk(t, "top"))
	lower0 = canonicalSource(t, mk(t, "lower0"))
	lower1 = NewDataSource[vroot.File](mk(t, "lower1"))
	t.Cleanup(func() { _ = lower1.Close() })

	f, err := New(top, []*DataSource{lower0, lower1}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, top, lower0, lower1
}

// link creates a symlink at name pointing at target in the layer's content
// tree, parents included.
func link(t *testing.T, d *DataSource, target string, name string) {
	t.Helper()
	path := d.ContentPath(name)
	if dir := filepath.Dir(path); dir != "." {
		if err := d.fsys.MkdirAll(dir, 0o777); err != nil {
			t.Fatalf("mkdirall %q: %v", dir, err)
		}
	}
	if err := d.fsys.Symlink(filepath.FromSlash(target), path); err != nil {
		t.Fatalf("symlink %q -> %q: %v", path, target, err)
	}
}

func read(t *testing.T, f *Fs, name string) string {
	t.Helper()
	file, err := f.Open(name)
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	defer func() { _ = file.Close() }()
	b, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read %q: %v", name, err)
	}
	return string(b)
}

func assertRead(t *testing.T, f *Fs, name string, want string) {
	t.Helper()
	if got := read(t, f, name); got != want {
		t.Errorf("read %q = %q, want %q", name, got, want)
	}
}

func assertErrIs(t *testing.T, op string, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s -> %v, want an error matching %v", op, err, want)
	}
}

// TestNewSweepsWork checks that whatever a previous run left staged is gone by
// the time New returns.
func TestNewSweepsWork(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		top := canonicalSource(t, mk(t, "top"))
		stale := top.StagingPath("tmp-1")
		create(t, top.fsys, stale)
		if err := top.fsys.Mkdir(top.StagingPath("leftover"), 0o777); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if _, err := New(top, nil, nil); err != nil {
			t.Fatalf("New: %v", err)
		}

		entries, err := vroot.ReadDir(top.fsys, dirWork)
		if err != nil {
			t.Fatalf("readdir work: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("work/ holds %d entries after New, want none", len(entries))
		}
	})
}

// TestNewRejects checks the two tops New refuses: a plain data source, which has
// nowhere to record masking, and a canonical one whose store opened read-only.
func TestNewRejects(t *testing.T) {
	t.Run("plain top", func(t *testing.T) {
		plain := NewDataSource[vroot.File](memfs.New("plain"))
		defer func() { _ = plain.Close() }()

		_, err := New(plain, nil, nil)
		assertErrIs(t, "New(plain)", err, ErrNotCanonical)
	})

	t.Run("read-only top", func(t *testing.T) {
		fsys := memfs.New("mem")
		written, err := NewDataSourceCanonical[vroot.File](fsys)
		if err != nil {
			t.Fatalf("NewDataSourceCanonical: %v", err)
		}
		if err := written.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		frozen, err := NewDataSourceCanonical[*vroot.ReadOnlyFile](
			vroot.NewReadOnlyFs[vroot.File](fsys),
		)
		if err != nil {
			t.Fatalf("NewDataSourceCanonical over a read-only fs: %v", err)
		}
		defer func() { _ = frozen.Close() }()

		_, err = New(frozen, nil, nil)
		assertErrIs(t, "New(read-only top)", err, ErrTopReadOnly)
		if errors.Is(err, ErrNotCanonical) {
			t.Error("a read-only top is reported as non-canonical; its layout is canonical")
		}
	})
}

// TestFsCloseOwnership checks which overlay owns the layers: the one New built
// closes them, and a sub-overlay that borrowed them leaves them open.
func TestFsCloseOwnership(t *testing.T) {
	store := newRecordingStore()
	top, err := NewDataSourceCanonicalStore[vroot.File](memfs.New("top"), store)
	if err != nil {
		t.Fatalf("NewDataSourceCanonicalStore: %v", err)
	}
	put(t, top, "sub/inside.txt", "in", 0o666)

	f, err := New(top, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sub, err := f.OpenRoot("sub")
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("sub.Close: %v", err)
	}
	if store.closed != 0 {
		t.Errorf("sub-overlay Close closed the shared store %d times", store.closed)
	}
	assertRead(t, f, filepath.FromSlash("sub/inside.txt"), "in")

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.closed != 1 {
		t.Errorf("overlay Close closed the store %d times, want 1", store.closed)
	}
}

// recordingStore is a MetadataStore kept in memory that counts the Close calls
// reaching it, which is how a layer's ownership is observed.
type recordingStore struct {
	whiteout map[string]struct{}
	opaque   map[string]struct{}
	closed   int
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		whiteout: make(map[string]struct{}),
		opaque:   make(map[string]struct{}),
	}
}

func (s *recordingStore) Load() ([]string, []string, error) {
	return slices.Sorted(maps.Keys(s.whiteout)), slices.Sorted(maps.Keys(s.opaque)), nil
}

func (s *recordingStore) SetWhiteout(name string) error {
	s.whiteout[cleanName(name)] = struct{}{}
	return nil
}

func (s *recordingStore) ClearWhiteout(name string) error {
	delete(s.whiteout, cleanName(name))
	return nil
}

func (s *recordingStore) SetOpaque(dir string) error {
	s.opaque[cleanName(dir)] = struct{}{}
	return nil
}

func (s *recordingStore) ClearOpaque(dir string) error {
	delete(s.opaque, cleanName(dir))
	return nil
}

func (s *recordingStore) Close() error {
	s.closed++
	return nil
}
