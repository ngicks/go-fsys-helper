package serr

import (
	"errors"
	"fmt"
	"testing"
)

func TestMultiError(t *testing.T) {
	for _, errs := range [][]error{
		{nil, nil},
		{},
		nil,
	} {
		assertNilInterface(t, NewMultiError(errs))
		assertNilInterface(t, NewMultiErrorChecked(errs))
		assertBool(t, NewMultiErrorUnchecked(errs) != nil, "not NewMultiErrorUnchecked(errs) != nil")
	}

	sampleErr1 := errors.New("errors")
	sampleErr2 := &exampleErr{"foo", "bar", "baz"}
	sampleErrs := []error{sampleErr1, nil, sampleErr2}

	for _, tc := range []struct {
		len int
		fn  func([]error) error
	}{
		{2, NewMultiError},
		{3, NewMultiErrorChecked},
		{3, NewMultiErrorUnchecked},
	} {
		err := tc.fn(sampleErrs)
		assertErrorsIs(t, err, sampleErr1)
		assertErrorsIs(t, err, sampleErr2)
		assertErrorsAs[*exampleErr](t, err)
		assertEq(t, tc.len, len(err.(interface{ Unwrap() []error }).Unwrap()))
	}

	type testCase struct {
		verb     string
		expected string
	}
	for _, tc := range []testCase{
		{verb: "%s", expected: "MultiError: errors, %!s(<nil>), exampleErr: Foo=foo Bar=bar Baz=baz"},
		{verb: "%v", expected: "MultiError: errors, <nil>, exampleErr: Foo=foo Bar=bar Baz=baz"},
		{verb: "%+v", expected: "MultiError: errors, <nil>, exampleErr: Foo=foo Bar=bar Baz=baz"},
		{verb: "%#v", expected: "MultiError: &errors.errorString{s:\"errors\"}, <nil>, &serr.exampleErr{Foo:\"foo\", Bar:\"bar\", Baz:\"baz\"}"},
		{verb: "%d", expected: "MultiError: &{%!d(string=errors)}, %!d(<nil>), &{%!d(string=foo) %!d(string=bar) %!d(string=baz)}"},
		{verb: "%T", expected: "*serr.multiError"},
		{verb: "%9.3f", expected: "MultiError: &{%!f(string=      err)}, %!f(<nil>), &{%!f(string=      foo) %!f(string=      bar) %!f(string=      baz)}"},
	} {
		tc := tc
		t.Run(tc.verb, func(t *testing.T) {
			e := NewMultiErrorUnchecked(sampleErrs)
			formatted := fmt.Sprintf(tc.verb, e)
			assertEq(t, tc.expected, formatted)
		})
	}

	nilMultiErr := NewMultiErrorUnchecked(nil)
	assertEq(t, "MultiError: ", nilMultiErr.Error())
}

type exampleErr struct {
	Foo string
	Bar string
	Baz string
}

func (e *exampleErr) Error() string {
	if e == nil {
		return "exampleErr: nil"
	}
	return fmt.Sprintf("exampleErr: Foo=%s Bar=%s Baz=%s", e.Foo, e.Bar, e.Baz)
}
