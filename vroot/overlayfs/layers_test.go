package overlayfs

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// mkLower builds a lower Layer over a fresh memfs named name, seeded by setup.
func mkLower(t *testing.T, name string, setup func(fsys vroot.Fs[vroot.File])) Layer {
	t.Helper()
	m := memfs.New(name)
	var fsys vroot.Fs[vroot.File] = m
	if setup != nil {
		setup(fsys)
	}
	return NewLayer(fsys, nil)
}

func mustWrite(t *testing.T, fsys vroot.Fs[vroot.File], name, content string) {
	t.Helper()
	f, err := fsys.Create(name)
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		t.Fatalf("write %q: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %q: %v", name, err)
	}
}

// TestLowerLstat_HighestPriorityWins verifies lowerLstat returns the entry from
// the highest-priority lower (the LAST slice element) when several have it, and
// fs.ErrNotExist when none do.
func TestLowerLstat_HighestPriorityWins(t *testing.T) {
	// lowers ordered low→high: low has shared+only-low; high has shared+only-high.
	low := mkLower(t, "low", func(fsys vroot.Fs[vroot.File]) {
		mustWrite(t, fsys, "shared.txt", "from-low")
		mustWrite(t, fsys, "only-low.txt", "low")
	})
	high := mkLower(t, "high", func(fsys vroot.Fs[vroot.File]) {
		mustWrite(t, fsys, "shared.txt", "from-high-and-longer")
		mustWrite(t, fsys, "only-high.txt", "high")
	})
	lowers := []Layer{low, high} // high is highest priority

	// shared.txt: highest-priority (high) wins. Distinguish by size.
	gotFsys, info, err := lowerLstat(lowers, "shared.txt")
	if err != nil {
		t.Fatalf("lowerLstat shared: %v", err)
	}
	if info.Size() != int64(len("from-high-and-longer")) {
		t.Errorf("shared.txt size = %d, want %d (high should win)",
			info.Size(), len("from-high-and-longer"))
	}
	if gotFsys != high.fsys {
		t.Errorf("shared.txt fsys is not the high lower")
	}

	// only-low.txt: only the low lower has it.
	gotFsys, _, err = lowerLstat(lowers, "only-low.txt")
	if err != nil {
		t.Fatalf("lowerLstat only-low: %v", err)
	}
	if gotFsys != low.fsys {
		t.Errorf("only-low.txt fsys is not the low lower")
	}

	// only-high.txt: only the high lower has it.
	gotFsys, _, err = lowerLstat(lowers, "only-high.txt")
	if err != nil {
		t.Fatalf("lowerLstat only-high: %v", err)
	}
	if gotFsys != high.fsys {
		t.Errorf("only-high.txt fsys is not the high lower")
	}

	// absent: fs.ErrNotExist.
	if _, _, err := lowerLstat(lowers, "nope.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lowerLstat absent err = %v, want fs.ErrNotExist", err)
	}

	// empty lowers: fs.ErrNotExist.
	if _, _, err := lowerLstat(nil, "anything"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lowerLstat empty err = %v, want fs.ErrNotExist", err)
	}
}

// TestLowerReadDir_UnionHigherWins verifies lowerReadDir unions names across
// lowers, with the higher-priority lower winning name collisions, and skips
// lowers where the dir is absent.
func TestLowerReadDir_UnionHigherWins(t *testing.T) {
	mkdir := func(fsys vroot.Fs[vroot.File], dir string) {
		if err := fsys.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}

	// three lowers low→high. "shared" exists in both low and mid but with a
	// DIFFERENT TYPE so the winner is identifiable via DirEntry.Type():
	// a regular file in low, a directory in mid (higher priority).
	low := mkLower(t, "low", func(fsys vroot.Fs[vroot.File]) {
		mkdir(fsys, "d")
		mustWrite(t, fsys, "d/a.txt", "low-a")
		mustWrite(t, fsys, "d/shared", "low-shared-as-file")
	})
	mid := mkLower(t, "mid", func(fsys vroot.Fs[vroot.File]) {
		mkdir(fsys, "d")
		mustWrite(t, fsys, "d/b.txt", "mid-b")
		mkdir(fsys, "d/shared") // shared is a DIRECTORY here
	})
	// high lacks dir "d" entirely — it must be skipped without error.
	high := mkLower(t, "high", func(fsys vroot.Fs[vroot.File]) {
		mustWrite(t, fsys, "elsewhere.txt", "high")
	})

	lowers := []Layer{low, mid, high}

	merged, err := lowerReadDir(lowers, "d")
	if err != nil {
		t.Fatalf("lowerReadDir d: %v", err)
	}

	names := slices.Sorted(maps.Keys(merged))
	want := []string{"a.txt", "b.txt", "shared"}
	if !slices.Equal(names, want) {
		t.Errorf("merged names = %v, want %v", names, want)
	}

	// shared: mid (higher priority than low) wins → it should be a directory,
	// not the regular file low contributes.
	if de, ok := merged["shared"]; !ok {
		t.Fatalf("shared missing from merge")
	} else if !de.IsDir() {
		t.Errorf("shared type = %v, want directory (mid should win over low)", de.Type())
	}

	// A dir no lower has → empty map, no error.
	emptyMerged, err := lowerReadDir(lowers, "missing-dir")
	if err != nil {
		t.Fatalf("lowerReadDir missing-dir: %v", err)
	}
	if len(emptyMerged) != 0 {
		t.Errorf("missing-dir merge = %v, want empty", emptyMerged)
	}
}

// TestLowerReadDir_SkipsNonDir verifies that a lower where the name is a file
// (not a directory) is skipped rather than erroring the whole merge.
func TestLowerReadDir_SkipsNonDir(t *testing.T) {
	// low has "x" as a FILE; high has "x" as a DIR with a child.
	low := mkLower(t, "low", func(fsys vroot.Fs[vroot.File]) {
		mustWrite(t, fsys, "x", "i-am-a-file")
	})
	high := mkLower(t, "high", func(fsys vroot.Fs[vroot.File]) {
		if err := fsys.Mkdir("x", 0o755); err != nil {
			t.Fatalf("mkdir x: %v", err)
		}
		mustWrite(t, fsys, "x/child.txt", "child")
	})
	lowers := []Layer{low, high}

	merged, err := lowerReadDir(lowers, "x")
	if err != nil {
		t.Fatalf("lowerReadDir x: %v", err)
	}
	names := slices.Sorted(maps.Keys(merged))
	if !slices.Equal(names, []string{"child.txt"}) {
		t.Errorf("merged names = %v, want [child.txt] (file-typed lower skipped)", names)
	}
}
