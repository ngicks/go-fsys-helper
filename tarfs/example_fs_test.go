package tarfs_test

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"

	"github.com/ngicks/go-fsys-helper/tarfs"
)

var treeTarGzBase64 = `H4sIAAAAAAAAA+2YTW7DIBBGWecUPoENZAbOg622yyg/VquevkNRlLRSf1gMScT3Nkg2EkjPD1mM` +
	`k1HHCpE5jy6yvR7PGEcxeLcN7OS5s+y9GVh/a8asx1M6DIN5Taf08vTzvL/ePyjjNM+z8jdQ5Z+9` +
	`+HdepsN/A4r/ZVkUv4Eq/zH3762F/yZc/O/X9U1njSw4EP3in7/7j178W53tfKVz/9n65tabADfj` +
	`un+l/P/RP136D5z7Z9qi/xbskX/XlP7ndFBco6p/Cvn/P5JD/y0Q8+i/Y879vyuuUdN/DO6zf3mN` +
	`/hsg5tF/x4xTSume7v+Iy/0f4f6nBcX/826nuEbd/x+X859w/rdAzOP8BwAAAAAAAIAO+ABC8URH` +
	`ACgAAA==`

// un-gzip embedded base64 string and returns tar binary.
func getTarReader() io.ReaderAt {
	treeTarGz, err := base64.StdEncoding.DecodeString(treeTarGzBase64)
	if err != nil {
		panic(err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(treeTarGz))
	if err != nil {
		panic(err)
	}
	tarBin, err := io.ReadAll(gr)
	if err != nil {
		panic(err)
	}
	err = gr.Close()
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(tarBin)
}

func Example_simple_usage() {
	fsys, err := tarfs.New(getTarReader(), nil)
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
