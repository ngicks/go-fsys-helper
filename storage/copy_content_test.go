package storage

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestCopyContents_validCopyContentsInput(t *testing.T) {
	type testCase struct {
		name string
		h    any
		c    any
		err  error
	}

	for _, tc := range []testCase{
		{
			name: "h is non struct",
			h:    20,
			c:    struct{}{},
			err:  ErrInvalidInput,
		},
		{
			name: "c is non struct",
			h:    struct{}{},
			c:    20,
			err:  ErrInvalidInput,
		},
		{
			name: "mismatched field number",
			h: pathHandle1{
				Foo: afero.NewMemMapFs(),
			},
			c:   contents2{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid dir handle",
			h:    invalidPathHandle{},
			c:    contents2{},
			err:  ErrInvalidInput,
		},
		{
			name: "invalid contents",
			h: pathHandle2{
				Foo: afero.NewMemMapFs(),
				Bar: afero.NewMemMapFs(),
			},
			c:   invalidContents{},
			err: ErrInvalidInput,
		},
		{
			name: "not same fields",
			h: pathHandle3{
				Foo: afero.NewMemMapFs(),
				Baz: afero.NewMemMapFs(),
			},
			c:   contents2{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid dir handle field",
			h:    pathHandle1{},
			c:    contents1{},
			err:  ErrInvalidInput,
		},
		{
			name: "valid",
			h: pathHandle1{
				Foo: afero.NewMemMapFs(),
			},
			c: contents1{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hRv := reflect.ValueOf(tc.h)
			cRv := reflect.ValueOf(tc.c)
			err := validCopyContentsInput(hRv, cRv, false)
			if tc.err == nil {
				assert.NilError(t, err)
			} else {
				assert.Assert(
					t,
					errors.Is(err, tc.err),
					"expected = %#v, actual = %#v",
					tc.err, err,
				)
			}
		})
	}
}

type contents1 struct {
	Foo fs.FS
}

type contents2 struct {
	Foo fs.FS
	Bar fs.FS
}

type invalidContents struct {
	Foo fs.FS
	Bar int
}

func TestCopyContents(t *testing.T) {

	t.Run("copy", func(t *testing.T) {
		handle := pathHandle2{
			Foo: afero.NewMemMapFs(),
			Bar: afero.NewMemMapFs(),
		}

		content := contents2{
			Foo: fstest.MapFS{
				"foo.env": &fstest.MapFile{
					Data:    []byte("FOO=foo"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
				"bar.env": &fstest.MapFile{
					Data:    []byte("BAR=bar"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
			},
			Bar: fstest.MapFS{
				"baz.env": &fstest.MapFile{
					Data:    []byte("BAZ=baz"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
				"qux": &fstest.MapFile{
					Data:    []byte("QUX=qux"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
			},
		}

		err := CopyContents(handle, content)
		assert.NilError(t, err)

		var result fsutil.EqualResult
		result, err = fsutil.Equal(afero.NewIOFS(handle.Foo), content.Foo)
		assert.NilError(t, err)
		assert.Assert(t, result.Equal())

		result, err = fsutil.Equal(afero.NewIOFS(handle.Bar), content.Bar)
		assert.NilError(t, err)
		assert.Assert(t, result.Equal())
	})

	t.Run("skip nil FS", func(t *testing.T) {
		handle := pathHandle2{
			Foo: afero.NewMemMapFs(),
			Bar: afero.NewMemMapFs(),
		}

		content := contents2{
			Foo: fstest.MapFS{
				"foo.env": &fstest.MapFile{
					Data:    []byte("FOO=foo"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
				"bar.env": &fstest.MapFile{
					Data:    []byte("BAR=bar"),
					Mode:    0o664,
					ModTime: time.Now(),
				},
			},
			// Bar is nil, skipped.
		}

		err := CopyContents(handle, content)
		assert.NilError(t, err)

		result, err := fsutil.Equal(afero.NewIOFS(handle.Foo), content.Foo)
		assert.NilError(t, err)
		assert.Assert(t, result.Equal())

		dirents, err := afero.ReadDir(handle.Bar, ".")
		assert.NilError(t, err)
		assert.Assert(t, len(dirents) == 0)
	})

}
