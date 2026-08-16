package overlayfs_test

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

var _ overlayfs.CopyUpPolicy = (*accessorPolicy)(nil)

// accessorPolicy is the copy-up a caller outside this package can write: it
// reaches the top through the exported accessors alone, staging under "work/"
// and publishing by rename the way the default policy does.
type accessorPolicy struct {
	copied []string
}

func (p *accessorPolicy) CopyUp(
	from vroot.Fs[vroot.File],
	top *overlayfs.DataSource,
	name string,
) error {
	info, err := from.Lstat(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	fsys := top.Fsys()
	if info.IsDir() {
		return fsys.Mkdir(top.ContentPath(name), info.Mode().Perm())
	}

	src, err := from.Open(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	staging := top.StagingPath("copy-" + strconv.Itoa(len(p.copied)))
	tmp, err := fsys.OpenFile(staging, os.O_RDWR|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := fsys.Rename(staging, top.ContentPath(name)); err != nil {
		return err
	}
	p.copied = append(p.copied, name)
	return nil
}

// TestCopyUpPolicyAccessors runs an out-of-package policy through
// [overlayfs.Option.CopyUpPolicy]: the file it stages and publishes with the
// exported accessors lands in the top's content tree, leaves nothing in "work/",
// and is what the overlay reads afterwards.
func TestCopyUpPolicyAccessors(t *testing.T) {
	var topFsys vroot.Fs[vroot.File] = memfs.New("top")
	top, err := overlayfs.NewDataSourceCanonical[vroot.File](topFsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}

	var lowerFsys vroot.Fs[vroot.File] = memfs.New("lower")
	testhelper.New[*testing.T, vroot.File](t, lowerFsys).SetupLines(`file.txt: "lower"`)
	lower := overlayfs.NewDataSource[vroot.File](lowerFsys)

	policy := &accessorPolicy{}
	f, err := overlayfs.New(
		top,
		[]*overlayfs.DataSource{lower},
		&overlayfs.Option{CopyUpPolicy: policy},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := f.Chmod("file.txt", 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if want := []string{"file.txt"}; !slices.Equal(policy.copied, want) {
		t.Errorf("the policy copied %q, want %q", policy.copied, want)
	}
	got, err := vroot.ReadFile(topFsys, top.ContentPath("file.txt"))
	if err != nil {
		t.Fatalf("read the top's copy: %v", err)
	}
	if string(got) != "lower" {
		t.Errorf("the top's copy holds %q, want %q", got, "lower")
	}
	staged, err := vroot.ReadDir(topFsys, "work")
	if err != nil {
		t.Fatalf("read work: %v", err)
	}
	if len(staged) != 0 {
		t.Errorf("work/ holds %d entries after the copy-up, want none", len(staged))
	}
	assertContent(t, f, "file.txt", "lower")
}
