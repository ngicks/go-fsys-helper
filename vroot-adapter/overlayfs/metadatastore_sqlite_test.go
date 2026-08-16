package overlayfs_test

import (
	"strconv"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/internal/sqlvfs"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

const storeName = "meta.sqlite3"

type backing struct {
	name string
	// open binds an opener to one fresh fsys, so every store it opens addresses
	// the same database file.
	open func(tb testing.TB) acceptancetest.OpenStore
}

// backings lists the fsys implementations the store must work over: memfs,
// which is type-erased and whose files cannot lock, and osfs, which has a
// concrete file type and locks through vroot.Locker.
func backings() []backing {
	return []backing{
		{"memfs", memfsBacking},
		{"osfs", osfsBacking},
	}
}

func memfsBacking(tb testing.TB) acceptancetest.OpenStore {
	fsys := memfs.New("mem")
	return func(tb testing.TB) overlayfs.MetadataStore {
		store, err := overlayfs.NewMetadataStoreSQLite[vroot.File](fsys, storeName)
		if err != nil {
			tb.Fatalf("NewMetadataStoreSQLite over memfs: %v", err)
		}
		return store
	}
}

func osfsBacking(tb testing.TB) acceptancetest.OpenStore {
	fsys, err := osfs.NewFs(tb.TempDir())
	if err != nil {
		tb.Fatalf("osfs.NewFs: %v", err)
	}
	tb.Cleanup(func() { _ = fsys.Close() })
	return func(tb testing.TB) overlayfs.MetadataStore {
		store, err := overlayfs.NewMetadataStoreSQLite[*osfs.File](fsys, storeName)
		if err != nil {
			tb.Fatalf("NewMetadataStoreSQLite over osfs: %v", err)
		}
		return store
	}
}

func TestMetadataStoreSQLite(t *testing.T) {
	for _, b := range backings() {
		t.Run(b.name, func(t *testing.T) {
			acceptancetest.RunMetadataStore(t, b.open)
		})
	}
}

// TestMetadataStoreSQLiteVersionMismatch checks that a database carrying a
// foreign schema version is rejected instead of written to.
func TestMetadataStoreSQLiteVersionMismatch(t *testing.T) {
	fsys := memfs.New("mem")

	store, err := overlayfs.NewMetadataStoreSQLite[vroot.File](fsys, storeName)
	if err != nil {
		t.Fatalf("NewMetadataStoreSQLite: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err := sqlvfs.Open(fsys, storeName)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE schema_info SET value = '2' WHERE key = 'version'`); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	store, err = overlayfs.NewMetadataStoreSQLite[vroot.File](fsys, storeName)
	if err == nil {
		_ = store.Close()
		t.Fatal("NewMetadataStoreSQLite accepted schema version 2")
	}
}

// BenchmarkMetadataStoreSQLite_SetWhiteout measures the write-through cost of
// one masking change: a committed sqlite transaction per call, through the VFS
// and the fsys under it.
func BenchmarkMetadataStoreSQLite_SetWhiteout(b *testing.B) {
	for _, backing := range backings() {
		b.Run(backing.name, func(b *testing.B) {
			store := backing.open(b)(b)
			defer store.Close()

			for i := 0; b.Loop(); i++ {
				// Distinct paths spread over directories: every call inserts a row
				// rather than updating one, which is what a whiteout run does.
				name := "dir" + strconv.Itoa(i/64) + "/file" + strconv.Itoa(i)
				if err := store.SetWhiteout(name); err != nil {
					b.Fatalf("SetWhiteout(%q): %v", name, err)
				}
			}
		})
	}
}
