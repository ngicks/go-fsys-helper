package testhelper

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestNilErr(t *testing.T) {
	t.Run("nil passes", func(t *testing.T) {
		ft := newFakeT(t)
		NilErr(ft, nil)
		if ft.failed {
			t.Fatalf("NilErr(nil) should not fail; msg = %q", ft.msg)
		}
	})

	t.Run("non-nil fails", func(t *testing.T) {
		ft := newFakeT(t)
		NilErr(ft, errors.New("boom"))
		if !ft.failed {
			t.Fatalf("NilErr(non-nil) should fail")
		}
	})
}

func TestErrIs(t *testing.T) {
	target := errors.New("sentinel")
	wrapped := fmt.Errorf("wrap: %w", target)

	t.Run("direct match", func(t *testing.T) {
		ft := newFakeT(t)
		ErrIs(ft, target, target)
		if ft.failed {
			t.Fatalf("ErrIs should pass; msg = %q", ft.msg)
		}
	})

	t.Run("wrapped match", func(t *testing.T) {
		ft := newFakeT(t)
		ErrIs(ft, wrapped, target)
		if ft.failed {
			t.Fatalf("ErrIs should pass on wrapped err; msg = %q", ft.msg)
		}
	})

	t.Run("nil target matches nil err", func(t *testing.T) {
		ft := newFakeT(t)
		ErrIs(ft, nil, nil)
		if ft.failed {
			t.Fatalf("ErrIs(nil, nil) should pass; msg = %q", ft.msg)
		}
	})

	t.Run("mismatch fails", func(t *testing.T) {
		ft := newFakeT(t)
		ErrIs(ft, errors.New("other"), target)
		if !ft.failed {
			t.Fatalf("ErrIs should fail on mismatch")
		}
	})

	t.Run("non-nil err with nil target fails", func(t *testing.T) {
		ft := newFakeT(t)
		ErrIs(ft, target, nil)
		if !ft.failed {
			t.Fatalf("ErrIs(non-nil, nil) should fail")
		}
	})
}

func TestErrAsType(t *testing.T) {
	pe := &fs.PathError{Op: "stat", Path: "/x", Err: fs.ErrNotExist}
	wrapped := fmt.Errorf("wrap: %w", pe)

	t.Run("direct extracts", func(t *testing.T) {
		ft := newFakeT(t)
		got := ErrAsType[*fs.PathError](ft, pe)
		if ft.failed {
			t.Fatalf("ErrAsType should pass; msg = %q", ft.msg)
		}
		if got != pe {
			t.Fatalf("got = %v, want %v", got, pe)
		}
	})

	t.Run("wrapped extracts", func(t *testing.T) {
		ft := newFakeT(t)
		got := ErrAsType[*fs.PathError](ft, wrapped)
		if ft.failed {
			t.Fatalf("ErrAsType should pass on wrapped; msg = %q", ft.msg)
		}
		if got.Path != "/x" {
			t.Fatalf("got.Path = %q, want %q", got.Path, "/x")
		}
	})

	t.Run("type mismatch fails", func(t *testing.T) {
		ft := newFakeT(t)
		_ = ErrAsType[*fs.PathError](ft, errors.New("plain"))
		if !ft.failed {
			t.Fatalf("ErrAsType should fail on type mismatch")
		}
	})
}

func TestErrAsTypeAnd(t *testing.T) {
	pe := &fs.PathError{Op: "stat", Path: "/x", Err: fs.ErrNotExist}

	t.Run("callback true passes", func(t *testing.T) {
		ft := newFakeT(t)
		ErrAsTypeAnd[*fs.PathError](ft, pe, func(p *fs.PathError) bool {
			return p.Op == "stat"
		})
		if ft.failed {
			t.Fatalf("ErrAsTypeAnd should pass; msg = %q", ft.msg)
		}
	})

	t.Run("callback false fails", func(t *testing.T) {
		ft := newFakeT(t)
		ErrAsTypeAnd[*fs.PathError](ft, pe, func(p *fs.PathError) bool {
			return p.Op == "open"
		})
		if !ft.failed {
			t.Fatalf("ErrAsTypeAnd should fail when callback returns false")
		}
	})

	t.Run("type mismatch fails before callback", func(t *testing.T) {
		ft := newFakeT(t)
		called := false
		ErrAsTypeAnd[*fs.PathError](ft, errors.New("plain"), func(p *fs.PathError) bool {
			called = true
			return true
		})
		if !ft.failed {
			t.Fatalf("ErrAsTypeAnd should fail on type mismatch")
		}
		if called {
			t.Fatalf("callback should not be invoked on type mismatch")
		}
	})
}
