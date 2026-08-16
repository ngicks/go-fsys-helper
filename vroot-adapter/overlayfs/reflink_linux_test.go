package overlayfs

import (
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestReflinkFileOsBacked runs the clone over two real files. Whether FICLONE is
// there depends on the filesystem the test directory sits on — ext4 and tmpfs
// have none, btrfs and xfs do — so both answers pass, and only the one that
// claims to have cloned is held to having copied the data.
func TestReflinkFileOsBacked(t *testing.T) {
	const content = "reflink me"
	dir := t.TempDir()
	fsys := newOsfs(t, dir)

	if err := vroot.WriteFile(fsys, "src", []byte(content), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	src, err := fsys.Open("src")
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer src.Close()

	dst, err := fsys.OpenFile("dst", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dst.Close()

	if src.Fd() == ^uintptr(0) || dst.Fd() == ^uintptr(0) {
		t.Fatalf("osfs files report no descriptor src=%#x dst=%#x", src.Fd(), dst.Fd())
	}

	if !reflinkFile(dst, src) {
		t.Skipf("%s does not support FICLONE; copy-up falls back to io.CopyBuffer", dir)
	}
	if got := contentOf(t, fsys, "dst"); got != content {
		t.Errorf("cloned content = %q, want %q", got, content)
	}
}

// TestCopyUpPolicyWorkOsBacked copies a file up with both layers os-backed, the
// only pair reflink can take. It asserts the result rather than the route: the
// content and mode have to match whether the clone ran or the buffered loop did.
func TestCopyUpPolicyWorkOsBacked(t *testing.T) {
	const content = "os-backed content"
	top := canonicalSource(t, newOsfs(t, t.TempDir()))
	src := NewDataSource[vroot.File](newOsfs(t, t.TempDir()))
	t.Cleanup(func() { _ = src.Close() })
	put(t, src, "file.txt", content, 0o640)

	if err := NewCopyUpPolicyWork().CopyUp(view(t, src), top, "file.txt"); err != nil {
		t.Fatalf("CopyUp: %v", err)
	}

	path := top.ContentPath("file.txt")
	if got := contentOf(t, top.fsys, path); got != content {
		t.Errorf("copied content = %q, want %q", got, content)
	}
	assertMode(t, top.fsys, path, 0o640)
	assertWorkEmpty(t, top)
}
