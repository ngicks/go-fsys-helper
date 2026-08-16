package testhelper

import (
	"errors"
	"fmt"
)

// msgSuffix renders formatAndArgs into a suffix appended to assertion failure messages.
// formatAndArgs is interpreted like [github.com/stretchr/testify/assert]:
// a lone string, a format string followed by its args, or arbitrary values.
func msgSuffix(formatAndArgs []any) string {
	if len(formatAndArgs) == 0 {
		return ""
	}
	if format, ok := formatAndArgs[0].(string); ok {
		return ": " + fmt.Sprintf(format, formatAndArgs[1:]...)
	}
	return ": " + fmt.Sprint(formatAndArgs...)
}

// NilErr is shorthand for ErrIs(t, err, nil).
func NilErr[T Test[T]](t T, err error, formatAndArgs ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got: %v%s", err, msgSuffix(formatAndArgs))
	}
}

// ErrIs is a thin wrapper of [errors.Is] that fails the test when err does not match target.
//
// As a special case, target == nil checks that err is nil.
func ErrIs[T Test[T]](t T, err error, target error, formatAndArgs ...any) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error mismatch: got %v, want %v%s", err, target, msgSuffix(formatAndArgs))
	}
}

// ErrAsType is a thin wrapper of [errors.AsType] that fails the test when err is not
// assignable to E. When successful, it returns the unwrapped value.
func ErrAsType[E error, T Test[T]](t T, err error, formatAndArgs ...any) E {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		t.Fatalf(
			"error type mismatch: got %v (type %T), want type %T%s",
			err,
			err,
			target,
			msgSuffix(formatAndArgs),
		)
	}
	return target
}

// ErrAsTypeAnd is like [ErrAsType] but additionally invokes fn with the unwrapped value.
// The test fails when err is not assignable to E or when fn reports false.
func ErrAsTypeAnd[E error, T Test[T]](t T, err error, fn func(E) bool, formatAndArgs ...any) {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		t.Fatalf(
			"error type mismatch: got %v (type %T), want type %T%s",
			err,
			err,
			target,
			msgSuffix(formatAndArgs),
		)
		return
	}
	if !fn(target) {
		t.Fatalf(
			"error did not satisfy callback: %v (type %T)%s",
			err,
			target,
			msgSuffix(formatAndArgs),
		)
	}
}
