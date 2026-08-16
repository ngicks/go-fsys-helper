package overlayfs_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// stack is one fsys implementation every layer of an acceptance stack is built
// over, together with what the suite may expect of that implementation.
type stack struct {
	name string
	// fsys hands out a fresh backing for one layer. name is only what the layer
	// calls itself; backings that need storage of their own take it from t.
	fsys   func(t *testing.T, name string) vroot.Fs[vroot.File]
	option acceptancetest.Option
}

// stacks lists the backings the overlay must pass the suite over: memfs, which
// is in memory and type-erased, and osfs, which has a concrete file type, real
// ownership and the reflink path under it.
func stacks() []stack {
	return []stack{
		{
			name: "memfs",
			fsys: func(_ *testing.T, name string) vroot.Fs[vroot.File] { return memfs.New(name) },
			// Nothing is skipped, the option memfs itself runs the suite with:
			// memfs is pure Go, so symlinks work on every platform and Chown
			// stores the pair it is handed with no real ownership behind it.
			option: acceptancetest.Option{
				ChownUid: 1000,
				ChownGid: 1000,
			},
		},
		{
			name: "osfs",
			fsys: func(t *testing.T, _ string) vroot.Fs[vroot.File] {
				fsys, err := osfs.NewFs(t.TempDir())
				if err != nil {
					t.Fatalf("osfs.NewFs: %v", err)
				}
				return vroot.Widen(fsys)
			},
			// The option osfs itself runs the suite with: a real filesystem
			// answers to the process's own ids and to Windows' symlink privilege.
			option: acceptancetest.Option{
				SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
				SkipChown:   runtime.GOOS == "windows",
				ChownUid:    os.Getuid(),
				ChownGid:    os.Getgid(),
			},
		},
	}
}

// buildStack stacks a canonical top over a canonical lower and a plain one, all
// over s's backing, and hands back the merged view together with the two
// lowers' content roots.
//
// Both lower shapes are in the stack because they are addressed differently: a
// canonical layer's content sits under "merged/" and it carries a store, a plain
// one's content is its root and it carries nothing.
func buildStack(t *testing.T, s stack) (*overlayfs.Fs, [2]vroot.Fs[vroot.File]) {
	t.Helper()

	top, err := overlayfs.NewDataSourceCanonical[vroot.File](s.fsys(t, "top"))
	if err != nil {
		t.Fatalf("NewDataSourceCanonical(top): %v", err)
	}

	lower0Fsys := s.fsys(t, "lower0")
	lower0, err := overlayfs.NewDataSourceCanonical[vroot.File](lower0Fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical(lower0): %v", err)
	}
	// The view borrows the layer's fs and is never closed: closing it would close
	// the fs under it and with it the layer.
	lower0Content, err := vroot.NewPathPrefixFs(lower0Fsys, "merged")
	if err != nil {
		t.Fatalf("NewPathPrefixFs(lower0/merged): %v", err)
	}

	lower1Content := s.fsys(t, "lower1")
	lower1 := overlayfs.NewDataSource[vroot.File](lower1Content)

	f, err := overlayfs.New(top, []*overlayfs.DataSource{lower0, lower1}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, [2]vroot.Fs[vroot.File]{lower0Content, lower1Content}
}

// seedLowers materializes lines in the lowers rather than through the overlay,
// which is what makes the suite read across layers: every path it was handed
// resolves in a lower, and every write to one goes through copy-up.
//
// Directories go into both lowers, so a merged listing has to union them and
// drop the duplicates; files and symlinks alternate, so a directory's entries
// come from two layers at once.
func seedLowers(t *testing.T, content [2]vroot.Fs[vroot.File], lines []string) {
	t.Helper()
	var per [2][]string
	n := 0
	for _, line := range lines {
		if strings.HasSuffix(line, "/") {
			per[0] = append(per[0], line)
			per[1] = append(per[1], line)
			continue
		}
		per[n%2] = append(per[n%2], line)
		n++
	}
	for i, fsys := range content {
		testhelper.New[*testing.T, vroot.File](t, fsys).SetupLines(per[i]...)
	}
}

// makeSeeded builds the stack with lines materialized in the lowers.
func makeSeeded(t *testing.T, s stack, lines []string) *overlayfs.Fs {
	t.Helper()
	f, content := buildStack(t, s)
	seedLowers(t, content, lines)
	return f
}

// makeWritten builds the stack with lines materialized through the overlay
// itself: everything the suite then reads is in the top, where its own writes
// put it.
func makeWritten(t *testing.T, s stack, lines []string) *overlayfs.Fs {
	t.Helper()
	f, _ := buildStack(t, s)
	testhelper.New(t, f).SetupLines(lines...)
	return f
}

// TestStackWiring pins what makes the seeded suite worth running over the
// written one: the fixture is split over the two lowers, neither of which holds
// all of it, and the merged view puts it back together.
//
// Were the fixture to land in the top instead, every suite below would still
// pass while reading nothing but what it had just written.
func TestStackWiring(t *testing.T) {
	for _, s := range stacks() {
		t.Run(s.name, func(t *testing.T) {
			f, content := buildStack(t, s)
			defer func() { _ = f.Close() }()

			seedLowers(t, content, []string{
				"dir/",
				`dir/a.txt: "a"`,
				`dir/b.txt: "b"`,
			})

			assertContent(t, content[0], "dir/a.txt", "a")
			assertGone(t, content[0], "dir/b.txt")
			assertContent(t, content[1], "dir/b.txt", "b")
			assertGone(t, content[1], "dir/a.txt")
			assertDirNames(t, f, "dir", "a.txt", "b.txt")
		})
	}
}

// TestRoot runs the full [vroot.Root] suite — read, write, escapes and race —
// against an overlay whose fixture lives in the lowers.
func TestRoot(t *testing.T) {
	for _, s := range stacks() {
		t.Run(s.name, func(t *testing.T) {
			acceptancetest.RunRoot(t, acceptancetest.SetupRoot[vroot.File, *overlayfs.Fs]{
				Make: func(t *testing.T, lines []string) *overlayfs.Fs {
					return makeSeeded(t, s, lines)
				},
				Option: s.option,
			})
		})
	}
}

// TestFs runs the [vroot.Fs] suite against an overlay whose fixture was written
// through the overlay, the wiring where the top holds everything.
func TestFs(t *testing.T) {
	for _, s := range stacks() {
		t.Run(s.name, func(t *testing.T) {
			acceptancetest.RunFs(t, acceptancetest.Setup[vroot.File, *overlayfs.Fs]{
				Make: func(t *testing.T, lines []string) *overlayfs.Fs {
					return makeWritten(t, s, lines)
				},
				Option: s.option,
			})
		})
	}
}
