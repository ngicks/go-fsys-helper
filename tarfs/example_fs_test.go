package tarfs_test

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"

	"github.com/ngicks/go-fsys-helper/tarfs"
)

var (
	//go:embed testdata/muh/tree.tar
	treeBin []byte
)

func Example_simple_usage() {
	fsys, err := tarfs.New(bytes.NewReader(treeBin), nil)
	if err != nil {
		panic(err)
	}

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		f, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		// IMPORTANT: file implements io.ReaderAt and io.Seeker
		_ = f.(io.ReaderAt)
		_ = f.(io.Seeker)

		s, err := f.Stat()
		if err != nil {
			return err
		}

		var content []byte
		if !s.IsDir() {
			content, err = io.ReadAll(f)
			if err != nil {
				return err
			}
			// of course, file can seek back to anywhere
			_, err = f.(io.Seeker).Seek(int64(-len(content)+1), io.SeekCurrent)
			if err != nil {
				return err
			}
			content2, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			if !bytes.Equal(content[1:], content2) {
				return fmt.Errorf("not equal: %q != %q", string(content[1:]), string(content2))
			}
		}

		fmt.Printf("%q: %v", path, s)
		if !s.IsDir() {
			fmt.Printf(" %q", string(content))
		}
		fmt.Printf("\n")

		return nil
	})
	if err != nil {
		panic(err)
	}

	// Output:
	// ".": drwxr-xr-x 0 2025-03-06 06:08:24 ./
	// "aaa": drwxr-xr-x 0 2025-03-06 06:08:53 aaa/
	// "aaa/foo": -rw-r--r-- 4 2025-03-06 06:08:53 foo "foo\n"
	// "bbb": drwxr-xr-x 0 2025-03-06 06:08:58 bbb/
	// "bbb/bar": -rw-r--r-- 4 2025-03-06 06:08:54 bar "bar\n"
	// "bbb/baz": -rw-r--r-- 4 2025-03-06 06:11:13 baz "baz\n"
	// "bbb/ccc": drwxr-xr-x 0 2025-03-06 06:09:12 ccc/
	// "bbb/ccc/quux": -rw-r--r-- 5 2025-03-06 06:09:12 quux "quux\n"
	// "bbb/ccc/qux": -rw-r--r-- 4 2025-03-06 06:09:09 qux "qux\n"
}
