package testhelper

import "errors"

// NilErr is shorthand for ErrIs(t, err, nil).
func NilErr[T Test[T]](t T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// ErrIs is a thin wrapper of [errors.Is] that fails the test when err does not match target.
//
// As a special case, target == nil checks that err is nil.
func ErrIs[T Test[T]](t T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error mismatch: got %v, want %v", err, target)
	}
}

// ErrAsType is a thin wrapper of [errors.AsType] that fails the test when err is not
// assignable to E. When successful, it returns the unwrapped value.
func ErrAsType[E error, T Test[T]](t T, err error) E {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		t.Fatalf("error type mismatch: got %v (type %T), want type %T", err, err, target)
	}
	return target
}

// ErrAsTypeAnd is like [ErrAsType] but additionally invokes fn with the unwrapped value.
// The test fails when err is not assignable to E or when fn reports false.
func ErrAsTypeAnd[E error, T Test[T]](t T, err error, fn func(E) bool) {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		t.Fatalf("error type mismatch: got %v (type %T), want type %T", err, err, target)
		return
	}
	if !fn(target) {
		t.Fatalf("error did not satisfy callback: %v (type %T)", err, target)
	}
}
