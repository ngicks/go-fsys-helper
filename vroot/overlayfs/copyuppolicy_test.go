package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// writeFile creates a regular file at name with content and perm in fsys.
func writeFile(t *testing.T, fsys vroot.Fs[vroot.File], name, content string, perm fs.FileMode) {
	t.Helper()
	f, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
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
	if err := fsys.Chmod(name, perm); err != nil {
		t.Fatalf("chmod %q: %v", name, err)
	}
}

// readFile reads the whole content of name from fsys.
func readFile(t *testing.T, fsys vroot.Fs[vroot.File], name string) string {
	t.Helper()
	f, err := fsys.Open(name)
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	defer func() { _ = f.Close() }()
	var sb strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := f.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

// populateLower seeds from with a regular file, a directory, and a symlink, and
// returns the name of the regular file. perm is the regular file's mode.
func populateLower(t *testing.T, from vroot.Fs[vroot.File], regName string, perm fs.FileMode) {
	t.Helper()
	writeFile(t, from, regName, "lower-content", perm)
	if err := from.Mkdir("dir", 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	if err := from.Symlink("target/path", "link"); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}

// assertCopiedTypes runs CopyUp for the regular file, directory, and symlink
// populated by populateLower and checks each landed correctly in to.
func assertCopiedTypes(
	t *testing.T,
	policy CopyUpPolicy,
	from, to vroot.Fs[vroot.File],
	regName string,
	wantPerm fs.FileMode,
) {
	t.Helper()

	// regular file
	if err := policy.CopyUp(from, to, regName); err != nil {
		t.Fatalf("CopyUp regular %q: %v", regName, err)
	}
	if got := readFile(t, to, regName); got != "lower-content" {
		t.Errorf("copied content = %q, want %q", got, "lower-content")
	}
	info, err := to.Lstat(regName)
	if err != nil {
		t.Fatalf("lstat copied %q: %v", regName, err)
	}
	if got := info.Mode().Perm(); got != wantPerm.Perm() {
		t.Errorf("copied perm = %v, want %v", got, wantPerm.Perm())
	}

	// directory
	if err := policy.CopyUp(from, to, "dir"); err != nil {
		t.Fatalf("CopyUp dir: %v", err)
	}
	di, err := to.Lstat("dir")
	if err != nil {
		t.Fatalf("lstat copied dir: %v", err)
	}
	if !di.IsDir() {
		t.Errorf("copied dir is not a directory: mode=%v", di.Mode())
	}

	// symlink
	if err := policy.CopyUp(from, to, "link"); err != nil {
		t.Fatalf("CopyUp symlink: %v", err)
	}
	target, err := to.ReadLink("link")
	if err != nil {
		t.Fatalf("readlink copied: %v", err)
	}
	if target != "target/path" {
		t.Errorf("copied symlink target = %q, want %q", target, "target/path")
	}
}

func TestCopyUpPolicyDotTmp_AllTypes(t *testing.T) {
	from := memfs.New("lower")
	to := memfs.New("top")
	populateLower(t, from, "file.txt", 0o640)

	policy := NewCopyUpPolicyDotTmp(0)
	assertCopiedTypes(t, policy, from, to, "file.txt", 0o640)

	// No standing *.copyup.tmp left beside the destination after a clean copy.
	assertNoLeftoverTemp(t, to, ".")
}

func TestCopyUpPolicyStage_AllTypes(t *testing.T) {
	from := memfs.New("lower")
	to := memfs.New("top")
	populateLower(t, from, "file.txt", 0o640)

	policy := NewCopyUpPolicyStage("")
	assertCopiedTypes(t, policy, from, to, "file.txt", 0o640)

	// Work dir was created.
	if di, err := to.Lstat(DefaultWorkDir); err != nil || !di.IsDir() {
		t.Fatalf("work dir %q not created: info=%v err=%v", DefaultWorkDir, di, err)
	}
	// No standing temp left inside the work dir after a clean copy.
	assertNoLeftoverTemp(t, to, DefaultWorkDir)
}

// assertNoLeftoverTemp asserts no entry ending in copyupTempSuffix remains in
// dir of fsys.
func assertNoLeftoverTemp(t *testing.T, fsys vroot.Fs[vroot.File], dir string) {
	t.Helper()
	dirents, err := vroot.ReadDir(fsys, dir)
	if err != nil {
		t.Fatalf("readdir %q: %v", dir, err)
	}
	for _, de := range dirents {
		if strings.HasSuffix(de.Name(), copyupTempSuffix) {
			t.Errorf("leftover copy-up temp %q in %q", de.Name(), dir)
		}
	}
}

func TestCopyUpPolicyStage_SweepsLeftover(t *testing.T) {
	from := memfs.New("lower")
	to := memfs.New("top")
	writeFile(t, from, "file.txt", "lower-content", 0o644)

	// Pre-create the work dir with a planted leftover from a "crashed" run.
	if err := to.MkdirAll(DefaultWorkDir, 0o777); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	leftover := DefaultWorkDir + "/foo.0123456789.copyup.tmp"
	writeFile(t, to, leftover, "stale", 0o644)
	// Also plant a non-temp file that must survive the sweep.
	keep := DefaultWorkDir + "/keep.txt"
	writeFile(t, to, keep, "keep", 0o644)

	policy := NewCopyUpPolicyStage("")
	if err := policy.CopyUp(from, to, "file.txt"); err != nil {
		t.Fatalf("CopyUp: %v", err)
	}

	// Leftover swept.
	if _, err := to.Lstat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("leftover %q not swept: err=%v", leftover, err)
	}
	// Non-temp file preserved.
	if _, err := to.Lstat(keep); err != nil {
		t.Errorf("non-temp %q wrongly removed: err=%v", keep, err)
	}
	// Destination published, no fresh temp remains.
	if got := readFile(t, to, "file.txt"); got != "lower-content" {
		t.Errorf("published content = %q, want %q", got, "lower-content")
	}
	assertNoLeftoverTemp(t, to, DefaultWorkDir)
}

func TestCopyUpPolicyStage_PrepareOnce(t *testing.T) {
	from := memfs.New("lower")
	to := memfs.New("top")
	writeFile(t, from, "a.txt", "a", 0o644)
	writeFile(t, from, "b.txt", "b", 0o644)

	policy := NewCopyUpPolicyStage("")
	if err := policy.CopyUp(from, to, "a.txt"); err != nil {
		t.Fatalf("CopyUp a: %v", err)
	}
	// Plant a leftover AFTER prepare ran once; a second CopyUp must NOT sweep
	// it (prepare/sweep is once-only).
	leftover := DefaultWorkDir + "/planted.9999999999.copyup.tmp"
	writeFile(t, to, leftover, "planted", 0o644)
	if err := policy.CopyUp(from, to, "b.txt"); err != nil {
		t.Fatalf("CopyUp b: %v", err)
	}
	if _, err := to.Lstat(leftover); err != nil {
		t.Errorf("second CopyUp unexpectedly swept %q: err=%v", leftover, err)
	}
}

// TestCopyUp_LongName exercises TruncateUTF8 fitting: a ~300-byte multibyte
// source base name must still publish to the full original name.
func TestCopyUp_LongName(t *testing.T) {
	// "あ" is 3 bytes in UTF-8; 120 of them is 360 bytes, > 255.
	longBase := strings.Repeat("あ", 120)
	if len(longBase) <= defaultNameMax {
		t.Fatalf("test setup: long base only %d bytes", len(longBase))
	}

	for _, tc := range []struct {
		name   string
		policy CopyUpPolicy
	}{
		{"DotTmp", NewCopyUpPolicyDotTmp(0)},
		{"Stage", NewCopyUpPolicyStage("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := memfs.New("lower")
			to := memfs.New("top")
			writeFile(t, from, longBase, "long-content", 0o644)

			if err := tc.policy.CopyUp(from, to, longBase); err != nil {
				t.Fatalf("CopyUp long name: %v", err)
			}
			if got := readFile(t, to, longBase); got != "long-content" {
				t.Errorf("content = %q, want %q", got, "long-content")
			}
		})
	}
}

func TestCopyUp_UnsupportedType(t *testing.T) {
	// A stub source whose Lstat reports a named pipe — an unsupported type.
	from := &fifoFs{name: "fifo"}
	to := memfs.New("top")

	for _, tc := range []struct {
		name   string
		policy CopyUpPolicy
	}{
		{"DotTmp", NewCopyUpPolicyDotTmp(0)},
		{"Stage", NewCopyUpPolicyStage("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.CopyUp(from, to, "fifo")
			if !errors.Is(err, ErrTypeNotSupported) {
				t.Errorf("CopyUp fifo err = %v, want ErrTypeNotSupported", err)
			}
		})
	}
}

func TestCopyupTempPattern_FitsNameMax(t *testing.T) {
	longBase := strings.Repeat("あ", 120) // 360 bytes
	pattern := copyupTempPattern(longBase, defaultNameMax)
	// Worst-case rendered component: "*" expands to 10 digits.
	rendered := strings.Replace(pattern, "*", "0123456789", 1)
	if len(rendered) > defaultNameMax {
		t.Errorf("rendered temp name %d bytes > nameMax %d", len(rendered), defaultNameMax)
	}
	if !strings.HasSuffix(pattern, copyupTempSuffix) {
		t.Errorf("pattern %q missing suffix %q", pattern, copyupTempSuffix)
	}
}

// fifoFs is a minimal vroot.Fs[vroot.File] whose Lstat reports name as a named
// pipe, used to exercise the ErrTypeNotSupported path. All other methods panic
// (they must not be reached for an unsupported type).
type fifoFs struct {
	vroot.Fs[vroot.File] // nil; embedded only to satisfy the interface surface
	name                 string
}

func (f *fifoFs) Lstat(name string) (fs.FileInfo, error) {
	if name != f.name {
		return nil, fs.ErrNotExist
	}
	return fifoInfo{name: name}, nil
}

type fifoInfo struct {
	name string
}

func (i fifoInfo) Name() string       { return i.name }
func (i fifoInfo) Size() int64        { return 0 }
func (i fifoInfo) Mode() fs.FileMode  { return fs.ModeNamedPipe | 0o644 }
func (i fifoInfo) ModTime() time.Time { return time.Time{} }
func (i fifoInfo) IsDir() bool        { return false }
func (i fifoInfo) Sys() any           { return nil }
