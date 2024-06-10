package stream

import (
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
	var errs []error
	for _, c := range r.closers {
		errs = append(errs, c.Close())
	}
	return serr.NewMultiErrorChecked(errs)
}
