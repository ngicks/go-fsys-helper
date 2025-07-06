package overlayfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func prepareLayers(tempDir string) (*Fs, func(t *testing.T)) {
	var closers []func() error

	var layers []Layer
	for i, baseDir := range []string{
		"top",
		"layer1",
		"layer2",
		"layer3",
		"layer4",
	} {
		for _, funcDir := range []string{"meta", "merged"} {
			err := os.MkdirAll(filepath.Join(tempDir, baseDir, funcDir), fs.ModePerm)
			if err != nil {
				panic(err)
			}
		}
		dir := filepath.Join(tempDir, baseDir, "merged")
		switch i {
		case 1:
			// only regular files under root/readable/
			for d := range testhelper.FilterLineDirection(
				func(l testhelper.LineDirection) bool {
					return (l.LineKind == testhelper.LineKindMkdir) ||
						(l.LineKind == testhelper.LineKindWriteFile &&
							strings.HasPrefix(l.Path, filepath.FromSlash("root/readable/")) &&
							strings.Count(l.Path, string(filepath.Separator)) == 2)
				},
				slices.Values(acceptancetest.RootFsysDirections),
			) {
				d.MustExecuteOs(dir)
			}
			err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash("root/readable/subdir")))
			if err != nil {
				panic(err)
			}
		case 2:
			// only regular files under root/readable/
			for d := range testhelper.FilterLineDirection(
				func(l testhelper.LineDirection) bool {
					return (l.LineKind == testhelper.LineKindMkdir) ||
						(l.LineKind == testhelper.LineKindSymlink &&
							strings.HasPrefix(l.Path, filepath.FromSlash("root/readable/")) &&
							strings.Count(l.Path, string(filepath.Separator)) == 2)
				},
				slices.Values(acceptancetest.RootFsysDirections),
			) {
				d.MustExecuteOs(dir)
			}
			err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash("root/readable/subdir")))
			if err != nil {
				panic(err)
			}
		case 3:
			// only content under "root/readable/subdir"
			for d := range testhelper.FilterLineDirection(
				func(l testhelper.LineDirection) bool {
					return (l.LineKind == testhelper.LineKindMkdir) ||
						((l.LineKind == testhelper.LineKindWriteFile || l.LineKind == testhelper.LineKindSymlink) &&
							strings.HasPrefix(l.Path, filepath.FromSlash("root/readable/")) &&
							strings.Count(l.Path, string(filepath.Separator)) == 3)
				},
				slices.Values(acceptancetest.RootFsysDirections),
			) {
				d.MustExecuteOs(dir)
			}
			err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash("root/readable/subdir/double_nested/")))
			if err != nil {
				panic(err)
			}
		case 4:
			// only content under "root/readable/subdir"
			for d := range testhelper.FilterLineDirection(
				func(l testhelper.LineDirection) bool {
					return (l.LineKind == testhelper.LineKindMkdir) ||
						((l.LineKind == testhelper.LineKindWriteFile || l.LineKind == testhelper.LineKindSymlink) &&
							strings.HasPrefix(l.Path, filepath.FromSlash("root/readable/")) &&
							strings.Count(l.Path, string(filepath.Separator)) == 4)
				},
				slices.Values(acceptancetest.RootFsysDirections),
			) {
				d.MustExecuteOs(dir)
			}
			// for this layer, place also writable side.
			acceptancetest.MakeOsFsys(dir, true, true)
		}
		var rootedFs vroot.Rooted
		if i == 0 {
			// Top layer should be writable
			rootedFs = must(
				osfs.NewRooted(
					filepath.Join(tempDir, baseDir, "merged"),
				),
			)
		} else {
			// Other layers are read-only
			rootedFs = vroot.NewReadOnlyRooted(
				must(
					osfs.NewRooted(
						filepath.Join(tempDir, baseDir, "merged"),
					),
				),
			)
		}
		meta := must(
			osfs.NewRooted(
				filepath.Join(tempDir, baseDir, "meta"),
			),
		)
		closers = append(closers, meta.Close)
		layers = append(
			layers,
			NewLayer(
				NewMetadataStoreSimpleText(meta),
				rootedFs,
			),
		)
	}

	return New(
			layers[0],
			layers[1:],
			nil,
		), func(t *testing.T) {
			t.Helper()
			for _, c := range closers {
				err := c()
				if err != nil {
					t.Errorf("meta close error: %v", err)
				}
			}
		}
}

func TestRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	rr, err := r.OpenRoot(filepath.FromSlash("root/readable"))
	if err != nil {
		panic(err)
	}
	rw, err := r.OpenRoot(filepath.FromSlash("root/writable"))
	if err != nil {
		panic(err)
	}

	t.Run("read-only", func(t *testing.T) {
		acceptancetest.RootedReadOnly(t, vroot.NewReadOnlyRooted(rr))
	})
	t.Run("read-write", func(t *testing.T) {
		acceptancetest.RootedReadWrite(t, rw)
	})
	t.Run("as-io-fs", func(t *testing.T) {
		fstest.TestFS(vroot.ToIoFsRooted(rr), acceptancetest.RootFsysReadableFiles...)
	})
}
