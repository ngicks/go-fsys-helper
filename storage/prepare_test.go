package storage

import (
	_ "embed"
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestPrepare_validPrepareInput(t *testing.T) {
	type testCase struct {
		name string
		s    any
		h    any
		err  error
	}

	for _, tc := range []testCase{
		{
			name: "both nil",
			s:    nil,
			h:    nil,
		},
		{
			name: "single field",
			s: dirSet1{
				Foo: "foo",
			},
			h: &pathHandle1{},
		},
		{
			name: "2 fields",
			s: dirSet2{
				Foo: "./foo",
				Bar: "./bar",
			},
			h: &pathHandle2{},
		},
		{
			name: "specifying absolute path",
			s: dirSet1{
				Foo: "/foo",
			},
			h:   &pathHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "specifying non local directory",
			s: dirSet1{
				Foo: "../foo",
			},
			h:   &pathHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "specifying empty path",
			s:    dirSet1{},
			h:    &pathHandle1{},
			err:  ErrInvalidInput,
		},
		{
			name: "pathHandle is not pointer",
			s: dirSet1{
				Foo: "foo",
			},
			h:   pathHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid dirSet",
			s: invalidDirSet{
				Foo: "foo",
				Bar: 12,
			},
			h:   &pathHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid pathHandle",
			s: dirSet1{
				Foo: "foo",
			},
			h:   &invalidPathHandle{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field num 1",
			s: dirSet1{
				Foo: "foo",
			},
			h:   &pathHandle2{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field num 2",
			s: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			h:   &pathHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field 1",
			s: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			h:   &pathHandle3{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field 2",
			s: dirSet3{
				Foo: "foo",
				Baz: "baz",
			},
			h:   &pathHandle2{},
			err: ErrInvalidInput,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validPrepareInput(reflect.ValueOf(tc.s), reflect.ValueOf(tc.h))
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

type dirSet1 struct {
	Foo string
}

type dirSet2 struct {
	Foo string
	Bar string
}

type dirSet3 struct {
	Foo string
	Baz string
}

type invalidDirSet struct {
	Foo string
	Bar int
}

type pathHandle1 struct {
	Foo afero.Fs
}

type pathHandle2 struct {
	Foo afero.Fs
	Bar afero.Fs
}

type pathHandle3 struct {
	Foo afero.Fs
	Baz afero.Fs
}

type invalidPathHandle struct {
	Foo afero.Fs
	Bar int
}
