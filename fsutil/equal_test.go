package fsutil

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"gotest.tools/v3/assert"
)

func init() {
	_ = os.Chmod("testdata/fs6/random1.txt", 0o777)
}

var (
	//go:embed testdata/random1.txt
	random1 []byte
	//go:embed testdata/random2.txt
	random2 []byte
)

func TestEqual_sameFile(t *testing.T) {
	mapFs := fstest.MapFS(map[string]*fstest.MapFile{
		"random1": {
			Data: random1,
		},
		"random1_2": {
			Data: random1,
		},
		"random1_last_12bytes_modified": {
			Data: modifyLastNBytes(random1, 12),
		},
		"random2": {
			Data: random2,
		},
	})

	for _, tc := range [][2]string{
		{"random1", "random1_2"},
		{"random2", "random2"},
	} {
		f1, _ := mapFs.Open(tc[0])
		f2, _ := mapFs.Open(tc[1])
		equal, err := sameFile(f1, f2)
		_ = f1.Close()
		_ = f2.Close()
		assert.NilError(t, err)
		assert.Assert(t, equal)
	}

	for _, tc := range [][2]string{
		{"random1", "random1_last_12bytes_modified"},
		{"random1", "random2"},
	} {
		f1, _ := mapFs.Open(tc[0])
		f2, _ := mapFs.Open(tc[1])
		equal, err := sameFile(f1, f2)
		_ = f1.Close()
		_ = f2.Close()
		assert.NilError(t, err)
		assert.Assert(t, !equal)
	}
}

func modifyLastNBytes(b []byte, n int) []byte {
	buf := make([]byte, n)
	for {
		_, err := io.ReadFull(rand.Reader, buf)
		if err != nil {
			panic(err)
		}
		if !bytes.Equal(b[len(b)-n:], buf) {
			break
		}
	}

	out := bytes.Clone(b)
	copy(out[len(out)-n:], buf)
	return out
}

func TestEqual(t *testing.T) {
	type pair struct {
		name string
		l, r fs.FS
	}

	//NOTE: we can not use embed.FS since it fakes mode bits.

	for _, p := range []pair{
		{
			"nominal",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs2"),
		},
	} {
		t.Run(p.name, func(t *testing.T) {
			eq, err := Equal(p.l, p.r)
			assert.NilError(t, err)
			assert.Assert(t, eq.Equal())
		})
	}

	for _, p := range []pair{
		{
			"just renamed",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs3"),
		},
		{
			"different mode bits",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs4"),
		},
		{
			"directory",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs5"),
		},
		{
			"directory_2",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs6"),
		},
		{
			"directory has different mode bits",
			os.DirFS("testdata/fs5"),
			os.DirFS("testdata/fs6"),
		},
		{
			"additional content",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs7"),
		},
		{
			"slightly modified content",
			os.DirFS("testdata/fs1"),
			os.DirFS("testdata/fs8"),
		},
	} {
		t.Run(p.name, func(t *testing.T) {
			eq, err := Equal(
				ignoreHiddenFile(p.r),
				ignoreHiddenFile(p.l),
			)
			assert.NilError(t, err)
			assert.Assert(t, !eq.Equal())
		})
	}
}
