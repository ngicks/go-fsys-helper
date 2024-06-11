package stream

import (
	"bytes"
	"context"
	"io"
	"testing"
	"testing/iotest"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func TestIotest(t *testing.T) {
	org, splitted := openPregenerated()
	content := must(io.ReadAll(org))
	t.Run("cancellable", func(t *testing.T) {
		testhelper.AssertNilInterface(
			t,
			iotest.TestReader(
				NewCancellable(context.Background(), bytes.NewReader(content)),
				content,
			),
		)
	})
	t.Run("multiReadCloser", func(t *testing.T) {
		seekBack(splitted...)
		testhelper.AssertNilInterface(
			t,
			iotest.TestReader(
				NewMultiReadCloser(splitted...),
				content,
			),
		)
	})
	t.Run("multiReadAtSeekCloser", func(t *testing.T) {
		seekBack(splitted...)
		// multiReadAtSeekCloser only relies on ReadAt, therefore seeking back is not necessary but anyway.
		testhelper.AssertNilInterface(
			t,
			iotest.TestReader(
				NewMultiReadAtSeekCloser(must(SizedReadersFromFileLike(splitted))),
				content,
			),
		)
	})
}
