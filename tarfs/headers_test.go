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

var knownTypeflags = []byte{
	tar.TypeReg,
	tar.TypeRegA,
	tar.TypeLink,
	tar.TypeSymlink,
	tar.TypeChar,
	tar.TypeBlock,
	tar.TypeDir,
	tar.TypeFifo,
	tar.TypeCont,
	tar.TypeXHeader,
	tar.TypeXGlobalHeader,
	tar.TypeGNUSparse,
	tar.TypeGNULongName,
	tar.TypeGNULongLink,
}

func isKnownTypeflag(b byte) bool {
	return slices.Contains(knownTypeflags, b)
}

// collects files under $(go env GOROOT)/src/archive/tar/testdata
// reads all files' content through [tar.Reader.Read], then compares what readers made by collectHeaders and makeReader read.
func Test_iterHeaders_makeReader(t *testing.T) {
	names, err := testTars()
	if err != nil {
		panic(err)
	}

	t.Logf("names = %#v", names)

	for _, name := range names {
		// Some of them takes too long time.
		// skip them.
		if slices.Contains([]string{"gnu-sparse-big.tar", "pax-sparse-big.tar"}, filepath.Base(name)) {
			t.Logf("skipped %q: too long", filepath.Base(name))
			continue
		}

		if err := tryOpeningTar(t, name); err != nil {
			t.Logf("skipped %q: was unable to open with std archive/tar: %v", filepath.Base(name), err)
			continue
		}

		t.Run(filepath.Base(name), func(t *testing.T) {
			read, err := collectContents(t, name)
			if err != nil {
				panic(err)
			}

			f, err := os.Open(name)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			headers, err := tryCollectHeaderOffsets(Sections(f))
			if err != nil {
				panic(err)
			}
			for _, k := range slices.Sorted(maps.Keys(headers)) {
				h := headers[k]
				if !isKnownTypeflag(h.h.Typeflag) {
					t.Logf("typeflag field value not defined in archive/tar: %q", h.h.Typeflag)
				}
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
				} else {
					if !isKnownTypeflag(h.h.Typeflag) {
						t.Logf("read: %q(%d)", ellipsis(read[k]), len(read[k]))
					}
				}
			}
		})
	}
}

func tryOpeningTar(t *testing.T, name string) error {
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	t.Logf("reading = %q", name)
	tr := tar.NewReader(f)
	for i := 0; ; i++ {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("*tar.Reader.Next at index %d: %w", i, err)
		}
		t.Logf("    entry = %q", h.Name)
	}
	return nil
}

func collectContents(t *testing.T, name string) (map[string][]byte, error) {
	t.Logf("opening %q", name)
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		return nil, err
	}
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
		t.Logf("    entry = %q", h.Name)

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
