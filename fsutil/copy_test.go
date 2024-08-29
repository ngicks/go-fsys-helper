package fsutil

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestCopy(t *testing.T) {
	type testCase struct {
		dirFs string
		opt   []CopyFsOption
	}
	for _, tc := range []testCase{
		{"testdata/fs1", []CopyFsOption{}},
		{"testdata/fs2", []CopyFsOption{}},
		{"testdata/fs3", []CopyFsOption{}},
		{"testdata/fs4", []CopyFsOption{}},
		{"testdata/fs5", []CopyFsOption{}},
		{"testdata/fs6", []CopyFsOption{}},
		{"testdata/fs7", []CopyFsOption{}},
		{"testdata/fs8", []CopyFsOption{}},
		{"testdata/fs6", []CopyFsOption{}},
		{"testdata/fs8", []CopyFsOption{}},
	} {
		t.Run(tc.dirFs, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp(
				"",
				fmt.Sprintf("fsutil-test-Copy-%s-*", strings.Replace(tc.dirFs, "/", "_", 1)),
			)
			if err != nil {
				panic(err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			dst := afero.NewBasePathFs(afero.NewOsFs(), tmpDir)
			src := ignoreHiddenFile(os.DirFS(tc.dirFs))

			err = CopyFS(dst, src, tc.opt...)
			assert.NilError(t, err)

			eq, err := Equal(src, afero.NewIOFS(dst), tc.opt...)
			assert.NilError(t, err)
			assert.Assert(t, eq.Equal())

			if len(tc.opt) == 0 {
				return
			}

			err = CleanDir(dst, ".")
			assert.NilError(t, err)
			err = CopyFS(dst, src)
			assert.NilError(t, err)
			eq, err = Equal(src, afero.NewIOFS(dst), tc.opt...)
			assert.NilError(t, err)
			assert.Assert(t, !eq.Equal())
		})
	}
}
