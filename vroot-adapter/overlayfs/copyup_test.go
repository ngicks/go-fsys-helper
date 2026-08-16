package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// sourceLayer is one spelling of the layer a copy-up reads from. Both have to
// work: the policy is handed a content-rooted view either way, and the two forms
// build that view differently.
type sourceLayer struct {
	name  string
	build func(t *testing.T, fsys vroot.Fs[vroot.File]) *DataSource
}

func sourceLayers() []sourceLayer {
	return []sourceLayer{
		{"plain", func(t *testing.T, fsys vroot.Fs[vroot.File]) *DataSource {
			d := NewDataSource[vroot.File](fsys)
			t.Cleanup(func() { _ = d.Close() })
			return d
		}},
		{"canonical", func(t *testing.T, fsys vroot.Fs[vroot.File]) *DataSource {
			return canonicalSource(t, fsys)
		}},
	}
}

// twoBackings builds two independent fs of one kind, one for the top and one for
// the layer it copies from.
func twoBackings(t *testing.T, b backing) (top, src vroot.Fs[vroot.File]) {
	t.Helper()
	return b.open(t)(t), b.open(t)(t)
}

func view(t *testing.T, d *DataSource) vroot.Fs[vroot.File] {
	t.Helper()
	v, err := d.contentView()
	if err != nil {
		t.Fatalf("contentView: %v", err)
	}
	return v
}

// put writes content at the layer's content path for name, creating whatever
// parents it needs, and sets the file's mode.
func put(t *testing.T, d *DataSource, name string, content string, perm fs.FileMode) {
	t.Helper()
	path := d.ContentPath(name)
	if dir := filepath.Dir(path); dir != "." {
		if err := d.fsys.MkdirAll(dir, 0o777); err != nil {
			t.Fatalf("mkdirall %q: %v", dir, err)
		}
	}
	f, err := d.fsys.Create(path)
	if err != nil {
		t.Fatalf("create %q: %v", path, err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		_ = f.Close()
		t.Fatalf("write %q: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %q: %v", path, err)
	}
	if err := d.fsys.Chmod(path, perm); err != nil {
		t.Fatalf("chmod %q: %v", path, err)
	}
}

func mkdirAt(t *testing.T, d *DataSource, name string, perm fs.FileMode) {
	t.Helper()
	path := d.ContentPath(name)
	if err := d.fsys.MkdirAll(path, 0o777); err != nil {
		t.Fatalf("mkdirall %q: %v", path, err)
	}
	if err := d.fsys.Chmod(path, perm); err != nil {
		t.Fatalf("chmod %q: %v", path, err)
	}
}

func contentOf(t *testing.T, fsys vroot.Fs[vroot.File], path string) string {
	t.Helper()
	b, err := vroot.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(b)
}

// assertMode compares permission bits, which windows does not keep: os.Chmod
// there records only whether the file is writable.
func assertMode(t *testing.T, fsys vroot.Fs[vroot.File], path string, want fs.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := fsys.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %q = %v, want %v", path, got, want)
	}
}

func assertWorkEmpty(t *testing.T, d *DataSource) {
	t.Helper()
	entries, err := vroot.ReadDir(d.fsys, dirWork)
	if err != nil {
		t.Fatalf("readdir %q: %v", dirWork, err)
	}
	if len(entries) == 0 {
		return
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	t.Errorf("%q holds %q, want it empty", dirWork, names)
}

func assertAbsent(t *testing.T, d *DataSource, name string) {
	t.Helper()
	path := d.ContentPath(name)
	if _, err := d.fsys.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lstat %q = %v, want it to not exist", path, err)
	}
}

// TestCopyUpPolicyWorkRegular copies a regular file up from both spellings of a
// source layer over both backings: the top gets the content and the mode, the
// source is left as it was, and nothing stays staged.
func TestCopyUpPolicyWorkRegular(t *testing.T) {
	const content = "lower content"
	for _, b := range backings() {
		for _, l := range sourceLayers() {
			t.Run(b.name+"/"+l.name, func(t *testing.T) {
				topFsys, srcFsys := twoBackings(t, b)
				top := canonicalSource(t, topFsys)
				src := l.build(t, srcFsys)
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

				srcPath := src.ContentPath("file.txt")
				if got := contentOf(t, src.fsys, srcPath); got != content {
					t.Errorf("source content = %q, want it unchanged", got)
				}
			})
		}
	}
}

// TestCopyUpPolicyWorkParents copies a file up from the bottom of a deep path:
// every parent appears in the top as a plain directory carrying the source
// layer's mode, and nothing beside the copied file comes with them.
func TestCopyUpPolicyWorkParents(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	src := NewDataSource[vroot.File](memfs.New("lower"))
	t.Cleanup(func() { _ = src.Close() })

	mkdirAt(t, src, "a", 0o750)
	mkdirAt(t, src, "a/b", 0o705)
	put(t, src, "a/b/deep.txt", "deep", 0o644)
	put(t, src, "a/b/sibling.txt", "sibling", 0o644)

	if err := NewCopyUpPolicyWork().CopyUp(view(t, src), top, "a/b/deep.txt"); err != nil {
		t.Fatalf("CopyUp: %v", err)
	}

	for name, want := range map[string]fs.FileMode{"a": 0o750, "a/b": 0o705} {
		path := top.ContentPath(name)
		info, err := top.fsys.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %q: %v", path, err)
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", path)
		}
		assertMode(t, top.fsys, path, want)
	}
	if got := contentOf(
		t,
		top.fsys,
		top.ContentPath("a/b/deep.txt"),
	); got != "deep" {
		t.Errorf("copied content = %q, want %q", got, "deep")
	}
	assertAbsent(t, top, "a/b/sibling.txt")
	assertWorkEmpty(t, top)
}

// TestCopyUpPolicyWorkDir copies a directory up: the directory itself lands in
// the top with its mode, and its children stay behind for the merged view to go
// on reading from the layer below.
func TestCopyUpPolicyWorkDir(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	src := NewDataSource[vroot.File](memfs.New("lower"))
	t.Cleanup(func() { _ = src.Close() })

	mkdirAt(t, src, "dir", 0o751)
	put(t, src, "dir/child.txt", "child", 0o644)
	mkdirAt(t, src, "dir/sub", 0o755)

	if err := NewCopyUpPolicyWork().CopyUp(view(t, src), top, "dir"); err != nil {
		t.Fatalf("CopyUp: %v", err)
	}

	path := top.ContentPath("dir")
	info, err := top.fsys.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
	assertMode(t, top.fsys, path, 0o751)
	assertAbsent(t, top, "dir/child.txt")
	assertAbsent(t, top, "dir/sub")
	assertWorkEmpty(t, top)
}

// TestCopyUpPolicyWorkSymlink copies a symlink up as a symlink pointing at the
// same target, rather than as whatever the target holds.
func TestCopyUpPolicyWorkSymlink(t *testing.T) {
	for _, b := range backings() {
		t.Run(b.name, func(t *testing.T) {
			topFsys, srcFsys := twoBackings(t, b)
			top := canonicalSource(t, topFsys)
			src := NewDataSource[vroot.File](srcFsys)
			t.Cleanup(func() { _ = src.Close() })

			put(t, src, "target.txt", "pointed at", 0o644)
			if err := src.fsys.Symlink("target.txt", src.ContentPath("link")); err != nil {
				t.Fatalf("symlink: %v", err)
			}

			if err := NewCopyUpPolicyWork().CopyUp(view(t, src), top, "link"); err != nil {
				t.Fatalf("CopyUp: %v", err)
			}

			path := top.ContentPath("link")
			info, err := top.fsys.Lstat(path)
			if err != nil {
				t.Fatalf("lstat %q: %v", path, err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("mode of %q = %v, want a symlink", path, info.Mode())
			}
			target, err := top.fsys.ReadLink(path)
			if err != nil {
				t.Fatalf("readlink %q: %v", path, err)
			}
			if target != "target.txt" {
				t.Errorf("target = %q, want %q", target, "target.txt")
			}
			assertAbsent(t, top, "target.txt")
			assertWorkEmpty(t, top)
		})
	}
}

var errSourceRead = errors.New("source read failed")

// haltingFs hands out source files that stop reading partway, to break a
// copy-up with its temp already staged.
type haltingFs struct {
	vroot.Fs[vroot.File]
	limit int
}

func (f haltingFs) Open(name string) (vroot.File, error) {
	inner, err := f.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &haltingFile{File: inner, remaining: f.limit}, nil
}

type haltingFile struct {
	vroot.File
	remaining int
}

func (f *haltingFile) Read(b []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, errSourceRead
	}
	n, err := f.File.Read(b[:min(len(b), f.remaining)])
	f.remaining -= n
	return n, err
}

// TestCopyUpPolicyWorkStagingCleanup breaks a copy-up midway through the data:
// the error comes back, the staging directory is left empty, and the top does
// not hold a half-written file under the name.
func TestCopyUpPolicyWorkStagingCleanup(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	src := NewDataSource[vroot.File](memfs.New("lower"))
	t.Cleanup(func() { _ = src.Close() })
	put(t, src, "file.txt", "much longer than the read limit", 0o644)

	from := haltingFs{Fs: view(t, src), limit: 4}
	err := NewCopyUpPolicyWork().CopyUp(from, top, "file.txt")
	if !errors.Is(err, errSourceRead) {
		t.Fatalf("CopyUp = %v, want one matching errSourceRead", err)
	}
	assertWorkEmpty(t, top)
	assertAbsent(t, top, "file.txt")
}

// deviceFs reports every file as a device, which memfs cannot create but a
// copy-up still has to refuse.
type deviceFs struct {
	vroot.Fs[vroot.File]
}

func (f deviceFs) Lstat(name string) (fs.FileInfo, error) {
	info, err := f.Fs.Lstat(name)
	if err != nil {
		return nil, err
	}
	return deviceInfo{info}, nil
}

type deviceInfo struct {
	fs.FileInfo
}

func (i deviceInfo) Mode() fs.FileMode {
	return i.FileInfo.Mode() | fs.ModeDevice
}

// TestCopyUpPolicyWorkTypeNotSupported checks that a file of a type vroot cannot
// create is reported rather than copied as something else, and that the refusal
// leaves the top untouched — not even the parents of the name it refused.
func TestCopyUpPolicyWorkTypeNotSupported(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	src := NewDataSource[vroot.File](memfs.New("lower"))
	t.Cleanup(func() { _ = src.Close() })
	put(t, src, "a/b/dev", "", 0o644)

	err := NewCopyUpPolicyWork().CopyUp(deviceFs{Fs: view(t, src)}, top, "a/b/dev")
	if !errors.Is(err, ErrTypeNotSupported) {
		t.Fatalf("CopyUp = %v, want one matching ErrTypeNotSupported", err)
	}
	assertWorkEmpty(t, top)
	assertAbsent(t, top, "a/b/dev")
	assertAbsent(t, top, "a")
}

// TestCopyUpPolicyWorkTempNames checks that repeated copy-ups do not collide
// over the staging name, and leave nothing of it behind.
func TestCopyUpPolicyWorkTempNames(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	src := NewDataSource[vroot.File](memfs.New("lower"))
	t.Cleanup(func() { _ = src.Close() })

	policy := NewCopyUpPolicyWork()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		put(t, src, name, name, 0o644)
		if err := policy.CopyUp(view(t, src), top, name); err != nil {
			t.Fatalf("CopyUp %q: %v", name, err)
		}
		if got := contentOf(t, top.fsys, top.ContentPath(name)); got != name {
			t.Errorf("content of %q = %q, want %q", name, got, name)
		}
	}
	assertWorkEmpty(t, top)
}

// TestFsCopyUpOsBacked writes to a lower-backed file through the overlay with
// every layer os-backed, the one pair reflink can take, so the write goes
// through the whole gate rather than through the policy alone.
//
// The route is not asserted: a filesystem without FICLONE falls back to the
// buffered loop (ext4 and tmpfs have none, btrfs and xfs do) and has to arrive
// at the same place. What is asserted is that the write landed in a copy — a
// clone shares its data with the file it was made from until one of the two is
// written to, and the layer below must not follow the top's write.
func TestFsCopyUpOsBacked(t *testing.T) {
	const (
		lower = "lower-content"
		upper = "upper-content"
	)
	name := filepath.FromSlash("dir/file.txt")
	f, top, lower0, _ := newOverlay(t, func(t *testing.T, _ string) vroot.Fs[vroot.File] {
		return newOsfs(t, t.TempDir())
	})
	put(t, lower0, "dir/file.txt", lower, 0o640)

	// Written over rather than truncated, and at the same length, so the bytes
	// the copy-up produced are the ones being overwritten.
	file, err := f.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	if _, err := file.Write([]byte(upper)); err != nil {
		_ = file.Close()
		t.Fatalf("write %q: %v", name, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %q: %v", name, err)
	}

	assertRead(t, f, name, upper)
	assertMode(t, top.fsys, top.ContentPath(name), 0o640)
	if got := contentOf(t, lower0.fsys, lower0.ContentPath(name)); got != lower {
		t.Errorf("the layer copied from now reads %q, want %q", got, lower)
	}
	assertWorkEmpty(t, top)
}

// TestReflinkFileNotOsBacked checks the gate reflink is behind: a file that
// reports no descriptor is never handed to the ioctl.
func TestReflinkFileNotOsBacked(t *testing.T) {
	fsys := memfs.New("mem")
	create(t, fsys, "src")
	create(t, fsys, "dst")

	src, err := fsys.Open("src")
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer src.Close()
	dst, err := fsys.OpenFile("dst", 0, 0)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dst.Close()

	if src.Fd() != ^uintptr(0) || dst.Fd() != ^uintptr(0) {
		t.Fatalf("memfs files report descriptors src=%#x dst=%#x", src.Fd(), dst.Fd())
	}
	if reflinkFile(dst, src) {
		t.Error("reflinkFile cloned files that are not os-backed")
	}
}
