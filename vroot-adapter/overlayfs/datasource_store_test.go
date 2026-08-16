package overlayfs_test

import (
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// memStore is a minimal [overlayfs.MetadataStore] holding its marks in maps,
// the kind of store [overlayfs.NewDataSourceCanonicalStore] exists for.
// TestMemStore runs it against the contract.
type memStore struct {
	whiteout map[string]bool
	opaque   map[string]bool
	closed   bool
}

func newMemStore() *memStore {
	return &memStore{whiteout: map[string]bool{}, opaque: map[string]bool{}}
}

// reopen hands back a store over the same marks, standing in for opening one
// backing a second time.
func (s *memStore) reopen() *memStore {
	return &memStore{whiteout: s.whiteout, opaque: s.opaque}
}

func (s *memStore) Load() (whiteouts []string, opaques []string, err error) {
	return slices.Collect(maps.Keys(s.whiteout)), slices.Collect(maps.Keys(s.opaque)), nil
}

func (s *memStore) SetWhiteout(name string) error {
	name = storePath(name)
	dropped := func(marked string, _ bool) bool { return isUnder(name, marked) }
	maps.DeleteFunc(s.whiteout, dropped)
	maps.DeleteFunc(s.opaque, dropped)
	delete(s.opaque, name)
	s.whiteout[name] = true
	return nil
}

func (s *memStore) ClearWhiteout(name string) error {
	delete(s.whiteout, storePath(name))
	return nil
}

func (s *memStore) SetOpaque(dir string) error {
	s.opaque[storePath(dir)] = true
	return nil
}

func (s *memStore) ClearOpaque(dir string) error {
	delete(s.opaque, storePath(dir))
	return nil
}

func (s *memStore) Close() error {
	s.closed = true
	return nil
}

func storePath(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}

// isUnder reports whether name sits strictly below dir.
func isUnder(dir, name string) bool {
	if dir == "" {
		return name != ""
	}
	return strings.HasPrefix(name, dir+"/")
}

func TestMemStore(t *testing.T) {
	acceptancetest.RunMetadataStore(t, func(tb testing.TB) acceptancetest.OpenStore {
		store := newMemStore()
		return func(tb testing.TB) overlayfs.MetadataStore {
			store = store.reopen()
			return store
		}
	})
}

// TestNewDataSourceCanonicalStore checks that the caller's store replaces the
// default one outright — no database is opened beside the layout — and that
// closing the data source closes it.
func TestNewDataSourceCanonicalStore(t *testing.T) {
	fsys := memfs.New("mem")
	store := newMemStore()

	d, err := overlayfs.NewDataSourceCanonicalStore[vroot.File](fsys, store)
	if err != nil {
		t.Fatalf("NewDataSourceCanonicalStore: %v", err)
	}

	for _, name := range []string{"merged", "work"} {
		info, err := fsys.Stat(name)
		if err != nil {
			t.Fatalf("stat %q: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", name)
		}
	}
	if _, err := fsys.Stat(storeName); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %q = %v, want a not-exist error", storeName, err)
	}

	if err := store.SetWhiteout("gone"); err != nil {
		t.Fatalf("SetWhiteout: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !store.closed {
		t.Error("Close left the caller's store open")
	}

	whiteouts, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(whiteouts, []string{"gone"}) {
		t.Errorf("whiteouts = %q, want %q", whiteouts, []string{"gone"})
	}
}
