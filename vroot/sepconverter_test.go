package vroot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestSepConverter(t *testing.T) {
	t.Run("ToOsPath", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir = %s", tempDir)
		acceptancetest.MakeOsFsys(tempDir, true, true)

		t.Run("acceptancetest", func(t *testing.T) {
			r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "writable"))
			if err != nil {
				panic(err)
			}
			defer r.Close()
			osPath := vroot.ToOsPathRooted(r)
			acceptancetest.RootedReadWrite(t, osPath)
		})

		t.Run("accessing with slash separated path", func(t *testing.T) {
			r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
			if err != nil {
				panic(err)
			}
			defer r.Close()

			osPath := vroot.ToOsPathRooted(r)

			assertSameDirnames := func(t *testing.T, expected []string, actual []fs.DirEntry) {
				t.Helper()

				slices.Sort(expected)

				var names []string
				for _, dirent := range actual {
					names = append(names, dirent.Name())
				}

				slices.Sort(names)

				if !slices.Equal(expected, names) {
					t.Errorf("not equal: expected != actual\nexpected:\n%#v\n\nactual:\n%#v", expected, names)
				}
			}

			dirents, err := vroot.ReadDir(osPath, "subdir/")
			if err != nil {
				t.Errorf("readdir failed with %v", err)
			}
			expected := []string{"double_nested", "nested_file.txt", "symlink_upward", "symlink_upward_escapes"}
			assertSameDirnames(t, expected, dirents)

			dirents, err = vroot.ReadDir(osPath, "subdir/double_nested/")
			if err != nil {
				t.Errorf("readdir failed with %v", err)
			}
			expected = []string{"double_nested.txt"}
			assertSameDirnames(t, expected, dirents)
		})
	})

	t.Run("ToFsValidPath", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir = %s", tempDir)
		acceptancetest.MakeOsFsys(tempDir, true, false)

		t.Run("acceptancetest", func(t *testing.T) {
			r := os.DirFS(filepath.Join(tempDir, "root", "readable"))
			fsValid := vroot.ToFsValidPathRooted(newNoPathConversionFs(r.(fs.ReadLinkFS)))
			acceptancetest.RootedReadOnly(t, fsValid)
		})
	})
}

var _ vroot.Rooted = (*noPathConversionFs)(nil)

// noPathConversionFs calls raw fsys wihout converting path.
// Only for ToFsValidPath testing.
type noPathConversionFs struct {
	raw fs.ReadLinkFS
	vroot.Fs
}

func newNoPathConversionFs(fsys fs.ReadLinkFS) vroot.Rooted {
	return &noPathConversionFs{fsys, vroot.FromIoFsRooted(fsys, "testing")}
}

func (fsys *noPathConversionFs) Rooted() {}

func (fsys *noPathConversionFs) Lstat(name string) (fs.FileInfo, error) {
	info, err := fsys.Fs.Lstat(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	_, err = fsys.raw.Lstat(name)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (fsys *noPathConversionFs) Open(name string) (vroot.File, error) {
	f, err := fsys.Fs.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	rawFile, err := fsys.raw.Open(name)
	if err != nil {
		f.Close()
		return nil, err
	}
	rawFile.Close()
	return f, nil
}

func (fsys *noPathConversionFs) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	return fsys.Open(name)
}

func (fsys *noPathConversionFs) OpenRoot(name string) (vroot.Rooted, error) {
	r, err := fsys.Fs.OpenRoot(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	r.Close()
	sub, err := fs.Sub(fsys.raw, name)
	if err != nil {
		return nil, err
	}
	return newNoPathConversionFs(sub.(fs.ReadLinkFS)), nil
}

func (fsys *noPathConversionFs) ReadLink(name string) (string, error) {
	s, err := fsys.Fs.ReadLink(filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	_, err = fsys.raw.ReadLink(name)
	if err != nil {
		return "", err
	}
	return s, nil
}

func (fsys *noPathConversionFs) Stat(name string) (fs.FileInfo, error) {
	info, err := fsys.Fs.Stat(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	_, err = fs.Stat(fsys.raw, name)
	if err != nil {
		return nil, err
	}
	return info, nil
}
