package overlayfs_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

func must1(err error) {
	if err != nil {
		panic(err)
	}
}

func must2[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func tree(fsys vroot.Fs) error {
	return vroot.WalkDir(
		fsys,
		".",
		&vroot.WalkOption{ResolveSymlink: false},
		func(path, realPath string, d fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == "." || d.IsDir() {
				return nil
			}
			switch {
			case d.Mode().IsRegular():
				fmt.Printf("%s\n", filepath.ToSlash(path))
			case d.Mode()&os.ModeSymlink != 0:
				linkTarget, err := fsys.ReadLink(path)
				if err != nil {
					return err
				}
				fmt.Printf("%s -> %s\n", filepath.ToSlash(path), filepath.ToSlash(linkTarget))
			}
			return nil
		},
	)
}

func Example_overlay_symlink() {
	tempDir := must2(os.MkdirTemp("", ""))

	for i := range 4 {
		layer := filepath.Join(tempDir, "layer"+strconv.FormatInt(int64(i), 10))
		must1(os.MkdirAll(filepath.Join(layer, "meta"), fs.ModePerm))
		must1(os.MkdirAll(filepath.Join(layer, "data"), fs.ModePerm))
	}

	// create leyred file system like this.
	//
	//                    +-------+
	// LAYER3:            | link2 |<-----+
	//                    +-------+      |
	//                      |            |
	//         +-------+    |        +-------+
	// LAYER2: | link3 |<---+        | link1 |
	//         +-------+             +-------+
	//             |
	//             |      +------+
	// LAYER1:     +----->| file |
	//                    +------+

	must1(os.MkdirAll(filepath.Join(tempDir, "layer3", "data", filepath.FromSlash("a/b/")), fs.ModePerm))
	must1(os.Symlink("../link3", filepath.Join(tempDir, "layer3", "data", filepath.FromSlash("a/b/link2"))))

	must1(os.MkdirAll(filepath.Join(tempDir, "layer2", "data", filepath.FromSlash("a/b/c")), fs.ModePerm))
	must1(os.Symlink("../link2", filepath.Join(tempDir, "layer2", "data", filepath.FromSlash("a/b/c/link1"))))
	must1(os.Symlink("./b/file", filepath.Join(tempDir, "layer2", "data", filepath.FromSlash("a/link3"))))

	must1(os.MkdirAll(filepath.Join(tempDir, "layer1", "data", filepath.FromSlash("a/b/")), fs.ModePerm))
	must1(os.WriteFile(filepath.Join(tempDir, "layer1", "data", filepath.FromSlash("a/b/file")), []byte("foobar"), fs.ModePerm))

	var closer []func() error
	defer func() {
		for _, c := range closer {
			err := c()
			if err != nil {
				fmt.Printf("meta fsys close error = %v\n", err)
			}
		}
	}()
	composeLayer := func(i int) overlayfs.Layer {
		metaFsys := must2(
			osfs.NewRooted(filepath.Join(tempDir, "layer"+strconv.FormatInt(int64(i), 10), "meta")),
		)
		closer = append(closer, metaFsys.Close)

		meta := overlayfs.NewMetadataStoreSimpleText(metaFsys)
		data := must2(
			osfs.NewRooted(filepath.Join(tempDir, "layer"+strconv.FormatInt(int64(i), 10), "data")),
		)
		return overlayfs.NewLayer(meta, data)
	}

	fsys := overlayfs.New(
		composeLayer(0),
		[]overlayfs.Layer{composeLayer(1), composeLayer(2), composeLayer(3)},
		nil,
	)

	must1(tree(vroot.FromIoFsRooted(os.DirFS(tempDir).(fs.ReadLinkFS), tempDir)))

	fmt.Println()

	bin, err := vroot.ReadFile(fsys, filepath.FromSlash("a/b/c/link1"))
	if err != nil {
		fmt.Printf("err = %v\n", err)
	} else {
		fmt.Printf("%q: %s\n", "a/b/c/link1", string(bin))
	}

	// Output:
	// layer1/data/a/b/file
	// layer2/data/a/b/c/link1 -> ../link2
	// layer2/data/a/link3 -> ./b/file
	// layer3/data/a/b/link2 -> ../link3
	//
	// "a/b/c/link1": foobar
}
