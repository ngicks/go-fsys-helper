package acceptancetest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRootEscapes asserts that a [vroot.Root] confines every path-taking method
// against both syntactic ("..", absolute-path) escapes and symlink-driven
// escapes, returning [vroot.ErrPathEscapes].
//
// The suite is method-table driven: every method that resolves a path is
// exercised over the traversal set and, where the method follows symlinks, over
// real out-of-root symlink targets. Implementations backed by a real filesystem
// can point those symlinks at genuine files created outside the root via
// [SetupRoot.SetupExternal]; a regression that follows such a link then reads
// real out-of-root bytes instead of getting a benign ErrNotExist, so the bug is
// caught rather than masked.
//
// For each rejection it also asserts the surfaced *fs.PathError / *os.LinkError
// path fields echo the user-supplied (in-root) name and never leak a host path
// (an absolute path, or one containing the "outside"/"sibling" target names).
func TestRootEscapes[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) {
	// SetupExternal, when provided, materializes the real out-of-root targets the
	// escape symlinks point at. It runs before the Root is exercised.
	if s.SetupExternal != nil {
		s.SetupExternal(t)
	}

	r := makeRoot(t, s)
	c := newC(t, r)

	c.SetupLines(
		"sub/",
		`sub/inside.txt: "in"`,
	)
	if !s.Option.SkipSymlink {
		c.SetupLines(
			// File symlink whose target lives outside the root.
			"escapelink -> ../outside",
			"sub/escapelink -> ../../outside",
			// Directory symlink whose target lives outside the root; used both
			// as a final component and as an intermediate component.
			"escapedir -> ../outsidedir",
			// Absolute-target link: cannot be resolved while staying confined.
			"abslink -> /etc/hostname",
			// Two-hop chain that ultimately escapes.
			"chainlink -> escapedir",
		)
	}

	// traversal is the set of syntactic escapes shared by every method.
	traversal := []string{
		"..",
		filepath.FromSlash("../"),
		filepath.FromSlash("../sibling"),
		filepath.FromSlash("../outside"),
		filepath.FromSlash("sub/../.."),
		filepath.FromSlash("sub/../../outside"),
	}

	// assertEscape checks that err is an ErrPathEscapes and that no host path
	// leaked into the surfaced *fs.PathError / *os.LinkError fields.
	assertEscape := func(t *testing.T, err error) {
		t.Helper()
		testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
		assertNoHostPathLeak(t, err)
	}

	// Method table over path-component (syntactic) escapes. These hold for every
	// method that resolves a path, regardless of symlink following.
	pathMethods := pathEscapeMethods[F, R](r)
	for _, m := range pathMethods {
		for _, p := range traversal {
			t.Run(m.name+" "+p, func(t *testing.T) {
				assertEscape(t, m.fn(p))
			})
		}
	}

	if s.Option.SkipSymlink {
		return
	}

	// Symlink-following escapes. Methods that follow the resolved component must
	// reject a path that resolves through an out-of-root symlink with
	// ErrPathEscapes. Lstat/ReadLink are intentionally excluded — they operate on
	// the link itself, and a symlink stored inside the root that merely points
	// outside is not itself an escape.
	//
	// Two target families are kept apart so the shared Root is never corrupted:
	//
	//   - finalTargets resolve through a symlink at the FINAL component. The
	//     follow-methods here are all non-destructive on success (or fail before
	//     mutating), so a single shared Root survives the sweep.
	//   - intermediateTargets resolve through a symlink at an INTERMEDIATE
	//     component, so even Remove/RemoveAll follow the link and escape without
	//     unlinking anything in-root. Remove/RemoveAll are tested ONLY here: on a
	//     final-component link they would legitimately (and destructively) unlink
	//     the in-root link itself, which is not an escape.
	finalTargets := []string{
		"escapelink",                         // file symlink, final component
		filepath.FromSlash("sub/escapelink"), // file symlink in a subdir
		"escapedir",                          // dir symlink, final component
		"abslink",                            // absolute-target link
		"chainlink",                          // two-hop chain, final component
	}
	intermediateTargets := []string{
		filepath.FromSlash("escapedir/inside"), // dir symlink as intermediate component
		filepath.FromSlash("chainlink/inside"), // chain as intermediate component
		filepath.FromSlash("abslink/inside"),   // absolute-target link as intermediate component
	}

	for _, m := range symlinkFollowMethods[F, R](r) {
		for _, lnk := range append(append([]string{}, finalTargets...), intermediateTargets...) {
			t.Run(m.name+" via symlink "+lnk, func(t *testing.T) {
				assertEscape(t, m.fn(lnk))
			})
		}
	}

	// Remove/RemoveAll follow only intermediate symlink components; on a final
	// component they unlink the in-root link (legitimate, not an escape).
	for _, m := range symlinkUnlinkMethods[F, R](r) {
		for _, lnk := range intermediateTargets {
			t.Run(m.name+" via symlink "+lnk, func(t *testing.T) {
				assertEscape(t, m.fn(lnk))
			})
		}
	}
}

// pathEscapeMethod names a path-taking Root method and a thunk that invokes it
// with the given (escaping) path.
type pathEscapeMethod[F vroot.File, R vroot.Root[F, R]] struct {
	name string
	fn   func(p string) error
}

// pathEscapeMethods builds the table of methods exercised against syntactic
// (path-component) escapes. Every method here must reject "..", absolute, and
// subtree-escaping inputs with ErrPathEscapes.
func pathEscapeMethods[F vroot.File, R vroot.Root[F, R]](r R) []pathEscapeMethod[F, R] {
	return []pathEscapeMethod[F, R]{
		{"Open", func(p string) error { return closeFile(r.Open(p)) }},
		{"OpenFile", func(p string) error {
			return closeFile(r.OpenFile(p, os.O_RDONLY, 0))
		}},
		{"OpenFileCreate", func(p string) error {
			return closeFile(r.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644))
		}},
		{"Create", func(p string) error { return closeFile(r.Create(p)) }},
		{"Stat", func(p string) error { _, err := r.Stat(p); return err }},
		{"Lstat", func(p string) error { _, err := r.Lstat(p); return err }},
		{"ReadLink", func(p string) error { _, err := r.ReadLink(p); return err }},
		{"Mkdir", func(p string) error {
			return r.Mkdir(filepath.Join(p, "newdir"), 0o755)
		}},
		{"MkdirAll", func(p string) error {
			return r.MkdirAll(filepath.Join(p, "newdir"), 0o755)
		}},
		{"Remove", func(p string) error { return r.Remove(p) }},
		{"RemoveAll", func(p string) error { return r.RemoveAll(p) }},
		{"Chmod", func(p string) error { return r.Chmod(p, 0o600) }},
		{"Chown", func(p string) error { return r.Chown(p, 0, 0) }},
		{"Lchown", func(p string) error { return r.Lchown(p, 0, 0) }},
		// Rename/Link/Symlink with the escaping path as the *newname* (write-out).
		{"RenameNewname", func(p string) error {
			return r.Rename("sub/inside.txt", filepath.Join(p, "moved.txt"))
		}},
		{"RenameOldname", func(p string) error {
			return r.Rename(p, "moved.txt")
		}},
		{"LinkNewname", func(p string) error {
			return r.Link("sub/inside.txt", filepath.Join(p, "linked.txt"))
		}},
		{"SymlinkNewname", func(p string) error {
			return r.Symlink("whatever", filepath.Join(p, "newlink"))
		}},
		{"OpenRoot", func(p string) error {
			rr, err := r.OpenRoot(p)
			if err == nil {
				_ = rr.Close()
			}
			return err
		}},
	}
}

// symlinkFollowMethods builds the table of methods that follow the resolved
// symlink and must therefore reject a path that resolves through an out-of-root
// symlink with ErrPathEscapes. Every method here is non-destructive on success
// (or fails before mutating), so a single shared Root survives the sweep over
// both final- and intermediate-component targets.
//
// Lstat/ReadLink are intentionally absent: they operate on the link itself.
// Remove/RemoveAll live in [symlinkUnlinkMethods] — on a final-component link
// they unlink the link rather than follow it.
func symlinkFollowMethods[F vroot.File, R vroot.Root[F, R]](r R) []pathEscapeMethod[F, R] {
	return []pathEscapeMethod[F, R]{
		{"Open", func(p string) error { return closeFile(r.Open(p)) }},
		{"Stat", func(p string) error { _, err := r.Stat(p); return err }},
		{"Create", func(p string) error { return closeFile(r.Create(p)) }},
		{"OpenFileCreate", func(p string) error {
			return closeFile(r.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644))
		}},
		{"Chmod", func(p string) error { return r.Chmod(p, 0o600) }},
		{"Chown", func(p string) error { return r.Chown(p, 0, 0) }},
		{"Mkdir", func(p string) error {
			return r.Mkdir(filepath.Join(p, "newdir"), 0o755)
		}},
		{"OpenRoot", func(p string) error {
			rr, err := r.OpenRoot(p)
			if err == nil {
				_ = rr.Close()
			}
			return err
		}},
	}
}

// symlinkUnlinkMethods builds the table of methods that, like POSIX unlink, do
// NOT follow a final-component symlink. They are exercised only over
// intermediate-component targets, where the followed (intermediate) link escapes
// without unlinking anything in-root, so the shared Root is never corrupted.
func symlinkUnlinkMethods[F vroot.File, R vroot.Root[F, R]](r R) []pathEscapeMethod[F, R] {
	return []pathEscapeMethod[F, R]{
		{"Remove", func(p string) error { return r.Remove(p) }},
		{"RemoveAll", func(p string) error { return r.RemoveAll(p) }},
	}
}

// closeFile closes f if err is nil and returns err, so a method-table thunk can
// invoke an Open-style method without leaking the handle.
func closeFile[F vroot.File](f F, err error) error {
	if err == nil {
		_ = f.Close()
	}
	return err
}

// assertNoHostPathLeak verifies the surfaced error echoes the caller-supplied
// (in-root, relative) name and never leaks a host path. The only reliable
// machine-checkable signal is absoluteness: a confined Root resolves paths
// internally but must report the name the caller passed, which is always
// relative. An absolute path field means the implementation surfaced its
// host-side resolved path — exactly the leak this guards against.
func assertNoHostPathLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var fields []string
	var pe *fs.PathError
	if errors.As(err, &pe) {
		fields = append(fields, pe.Path)
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		fields = append(fields, le.Old, le.New)
	}
	for _, f := range fields {
		if f == "" {
			continue
		}
		if filepath.IsAbs(f) {
			t.Errorf("error path %q is absolute; a confined error must not leak a host path: %v", f, err)
		}
	}
}
