package stream

import (
	"fmt"
	"io"

	"github.com/ngicks/go-fsys-helper/stream/internal/serr"
)

var _ io.ReadCloser = (*multiReadCloser[io.ReadCloser])(nil)

type multiReadCloser[T io.ReadCloser] struct {
	r       io.Reader
	closers []T
}

func NewMultiReadCloser[T io.ReadCloser](r ...T) io.ReadCloser {
	var readers []io.Reader
	for _, rr := range r {
		readers = append(readers, rr)
	}

	return &multiReadCloser[T]{
		r:       io.MultiReader(readers...),
		closers: r,
	}
}

func (r *multiReadCloser[T]) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *multiReadCloser[T]) Close() error {
	return closeFanOut(r.closers, func(c T) (error, bool) {
		// Every element is an io.ReadCloser, so close unconditionally.
		return c.Close(), true
	})
}

// closeFanOut closes each item in items by calling closeOne and gathers the
// resulting errors, each prefixed with its index ("index %d: "). closeOne
// returns the close error together with a flag reporting whether the item was
// closed at all: when the flag is false the item is skipped entirely and no
// entry (not even a nil one) is recorded for that index, matching the original
// behavior of the two Close fan-outs this helper unifies (multiReadCloser
// closes every element; multiReadAtSeekCloser closes only segments that
// implement io.Closer). The returned error is nil when every recorded close
// returned nil.
func closeFanOut[T any](items []T, closeOne func(T) (err error, closed bool)) error {
	var errs []serr.PrefixErr
	for i, item := range items {
		err, closed := closeOne(item)
		if !closed {
			continue
		}
		errs = append(errs, serr.PrefixErr{P: fmt.Sprintf("index %d: ", i), E: err})
	}
	return serr.GatherPrefixed(errs)
}
