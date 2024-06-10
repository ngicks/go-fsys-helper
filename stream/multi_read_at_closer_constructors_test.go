package stream

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

func openPregenerated() (org *os.File, splitted []*os.File) {
	var (
		err error
	)
	org, err = os.Open("./testdata/random_bytes")
	if err != nil {
		panic(err)
	}

	dirents, err := os.ReadDir("./testdata")
	if err != nil {
		panic(err)
	}
	for _, dirent := range dirents {
		if strings.HasPrefix(dirent.Name(), "random_bytes.") {
			f, err := os.Open(filepath.Join("./testdata", dirent.Name()))
			if err != nil {
				panic(err)
			}
			splitted = append(splitted, f)
		}
	}
	sort.Slice(splitted, func(i, j int) bool {
		return splitted[i].Name() < splitted[j].Name()
	})

	return org, splitted
}

func TestSizedReadersFromFileLike(t *testing.T) {
	for _, prep := range []func([]*os.File) []SizedReaderAt{
		func(f []*os.File) []SizedReaderAt {
			sizedReaders, err := SizedReadersFromFileLike(f)
			testhelper.AssertErrorsIs(t, err, nil)
			return sizedReaders
		},
		func(f []*os.File) []SizedReaderAt {
			var seg []*io.SectionReader
			for _, ff := range f {
				s, err := ff.Stat()
				testhelper.AssertErrorsIs(t, err, nil)
				seg = append(seg, io.NewSectionReader(ff, 0, s.Size()))
			}
			return SizedReadersFromReadAtSizer(seg)
		},
	} {
		org, splitted := openPregenerated()

		readers := prep(splitted)

		r := NewMultiReadAtSeekCloser(readers)

		bin, err := io.ReadAll(r)
		testhelper.AssertErrorsIs(t, err, nil)

		binOriginal, err := io.ReadAll(org)
		testhelper.AssertErrorsIs(t, err, nil)

		testhelper.AssertTrue(t, bytes.Equal(binOriginal, bin), "bytes.Equal returned false")
	}
}
