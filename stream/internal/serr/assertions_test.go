package serr

import (
	"errors"
	"testing"
)

func assertErrorsIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("errors.Is(err, target) returned false, err = %#v, target = %#v", err, target)
	}
}

// func assertNotErrorsIs(t *testing.T, err, target error) {
// 	t.Helper()
// 	if errors.Is(err, target) {
// 		t.Fatalf("errors.Is(err, target) returned true, err = %#v, target = %#v", err, target)
// 	}
// }

func assertErrorsAs[T any](t *testing.T, err error) {
	t.Helper()
	var e T
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(err, target) returned false, expected to be type %T, but is %#v", e, err)
	}
}

func assertNilInterface(t *testing.T, v any) {
	t.Helper()
	if v != nil {
		t.Fatalf("not nil: v = %#v, expected to be nil", v)
	}
}

func assertBool(t *testing.T, b bool, format string, mgsArgs ...any) {
	t.Helper()
	if !b {
		t.Fatalf(format, mgsArgs...)
	}
}

func assertEq[T comparable](t *testing.T, x, y T) {
	t.Helper()
	if x != y {
		t.Fatalf("not equal: left =\n%v,\n\nright =\n%v", x, y)
	}
}
