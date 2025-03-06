package tarfs

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"testing"
)

// collects files under $(go${VERSION_DESCRIBED_IN_go.mod} env GOROOT)/src/archive/tar/testdata
// reads all files' content through [tar.Reader.Read], then compares what readers made by collectHeaders and makeReader read.
func Test_collectHeaders_makeReader(t *testing.T) {
	names, err := testTars()
	if err != nil {
		panic(err)
	}

	for _, name := range names {
		// Some of them takes too long time.
		// skip them.
		if !isTarOopenable(name) || slices.Contains([]string{"gnu-not-utf8.tar", "gnu-sparse-big.tar", "pax-sparse-big.tar"}, filepath.Base(name)) {
			continue
		}

		t.Run(filepath.Base(name), func(t *testing.T) {
			read, err := collectContents(name)
			if err != nil {
				panic(err)
			}

			f, err := os.Open(name)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			headers, err := collectHeaders(f)
			if err != nil {
				panic(err)
			}
			for _, k := range slices.Sorted(maps.Keys(headers)) {
				h := headers[k]
				r := makeReader(f, h)
				bin, err := io.ReadAll(r)
				if err != nil {
					panic(err)
				}
				if !bytes.Equal(read[k], bin) {
					t.Errorf(
						`read content not equal
filename = %q
expected = %q(%d)
actual = %q(%d)

header = %#v

`,
						h.h.Name,
						ellipsis(read[k]), len(read[k]),
						ellipsis(bin), len(bin),
						h.h,
					)
				}
			}
		})
	}
}

func isTarOopenable(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		_, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return false
		}
	}
	return true
}

func collectContents(name string) (map[string][]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	s, _ := f.Stat()
	// use section reader for easier offset checking
	sr := io.NewSectionReader(f, 0, s.Size())
	ret := make(map[string][]byte)
	tr := tar.NewReader(sr)
	for {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return ret, err
		}

		bin, err := io.ReadAll(tr)
		if err != nil {
			return ret, fmt.Errorf("reading %q: %w", path.Clean(h.Name), err)
		}
		ret[path.Clean(h.Name)] = bin
	}
	return ret, nil
}

func ellipsis(b []byte) []byte {
	if len(b) > 500 {
		return append(b[:500:500], "..."...)
	}
	return b
}
