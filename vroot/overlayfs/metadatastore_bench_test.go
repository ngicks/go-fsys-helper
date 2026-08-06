package overlayfs_test

import (
	"strconv"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// benchSequentialRemoves models N sequential removes (each a SetWhiteout) plus
// a final Flush, exercising append + amortized compaction. With the append-log
// store the total work stays ~linear in N (vs O(N^2) for a full-rewrite store).
func benchSequentialRemoves(b *testing.B, mode overlayfs.SyncMode) {
	b.Helper()
	for b.Loop() {
		fsys := memfs.New("meta")
		store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: mode})
		const removes = 1000
		for j := range removes {
			if err := store.SetWhiteout("dir/file" + strconv.Itoa(j) + ".txt"); err != nil {
				b.Fatalf("SetWhiteout: %v", err)
			}
		}
		if err := store.Flush(); err != nil {
			b.Fatalf("Flush: %v", err)
		}
		if err := store.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}

func BenchmarkMetadataStoreLogSequentialRemovesBatched(b *testing.B) {
	benchSequentialRemoves(b, overlayfs.SyncBatched)
}

func BenchmarkMetadataStoreLogSequentialRemovesPerOp(b *testing.B) {
	benchSequentialRemoves(b, overlayfs.SyncPerOp)
}

func BenchmarkMetadataStoreLogSequentialRemovesNone(b *testing.B) {
	benchSequentialRemoves(b, overlayfs.SyncNone)
}

func BenchmarkMetadataStoreMemSequentialRemoves(b *testing.B) {
	for b.Loop() {
		store := overlayfs.NewMetadataStoreMem()
		const removes = 1000
		for j := range removes {
			if err := store.SetWhiteout("dir/file" + strconv.Itoa(j) + ".txt"); err != nil {
				b.Fatalf("SetWhiteout: %v", err)
			}
		}
	}
}
