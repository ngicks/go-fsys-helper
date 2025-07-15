package testhelper

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

type ExtendedT interface {
	T
	PushContext(key string, value any) ExtendedT
	PushOp(op string) ExtendedT
	PushPath(path string) ExtendedT
	RunE(name string, f func(t ExtendedT)) bool
}

type T interface {
	Chdir(dir string)
	Cleanup(f func())
	Context() context.Context
	Deadline() (deadline time.Time, ok bool)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Helper()
	Log(args ...any)
	Logf(format string, args ...any)
	Name() string
	Parallel()
	Run(name string, f func(t *testing.T)) bool
	Setenv(key string, value string)
	Skip(args ...any)
	SkipNow()
	Skipf(format string, args ...any)
	Skipped() bool
	TempDir() string
}

func ExtendT(t T) ExtendedT {
	return wrapT(t)
}

type keyValue struct {
	K string
	V any
}

type tWrapper struct {
	T
	stack []keyValue
}

func wrapT(t T) *tWrapper {
	if tt, ok := t.(*tWrapper); ok {
		return tt
	}
	return &tWrapper{
		T: t,
	}
}

func (t *tWrapper) PushContext(key string, value any) ExtendedT {
	return &tWrapper{
		T:     t.T,
		stack: append(slices.Clone(t.stack), keyValue{key, value}),
	}
}

func (t *tWrapper) PushOp(op string) ExtendedT {
	return t.PushContext("Op", op)
}

func (t *tWrapper) PushPath(path string) ExtendedT {
	return t.PushContext("Path", path)
}

func (t *tWrapper) RunE(name string, f func(t ExtendedT)) bool {
	t.Helper()
	return t.T.Run(
		name,
		func(t_ *testing.T) {
			tt := &tWrapper{
				T:     t_,
				stack: slices.Clone(t.stack),
			}
			f(tt)
		},
	)
}

func (t *tWrapper) msgPrefix() string {
	var builder strings.Builder
	builder.WriteString("\n")
	for _, ele := range t.stack {
		builder.WriteString(strings.ReplaceAll(ele.K, "%", "%%"))
		builder.WriteString(": ")
		builder.WriteString(strings.ReplaceAll(fmt.Sprintf("%v\n", ele.V), "%", "%%"))
	}
	builder.WriteString("\n")
	return builder.String()
}

func (t *tWrapper) Error(args ...any) {
	t.Helper()
	t.T.Error(append([]any{t.msgPrefix()}, args...))
}

func (t *tWrapper) Errorf(format string, args ...any) {
	t.Helper()
	t.T.Errorf(t.msgPrefix()+format, args...)
}

func (t *tWrapper) Fatal(args ...any) {
	t.Helper()
	t.T.Fatal(append([]any{t.msgPrefix()}, args...))
}

func (t *tWrapper) Fatalf(format string, args ...any) {
	t.Helper()
	t.T.Fatalf(t.msgPrefix()+format, args...)
}

func (t *tWrapper) Log(args ...any) {
	t.Helper()
	t.T.Log(append([]any{t.msgPrefix()}, args...))
}

func (t *tWrapper) Logf(format string, args ...any) {
	t.Helper()
	t.T.Logf(t.msgPrefix()+format, args...)
}

func (t *tWrapper) Skip(args ...any) {
	t.Helper()
	t.T.Skip(append([]any{t.msgPrefix()}, args...))
}

func (t *tWrapper) Skipf(format string, args ...any) {
	t.Helper()
	t.T.Skipf(t.msgPrefix()+format, args...)
}
