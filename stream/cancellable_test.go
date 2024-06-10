package stream

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

func TestCancellable(t *testing.T) {
	buf := make([]byte, 1024)
	t.Run("read_all", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancellable := NewCancellable(ctx, bytes.NewReader(randomBytes))
		bin, err := io.ReadAll(cancellable)
		testhelper.AssertErrorsIs(t, err, nil)
		testhelper.AssertTrue(t, bytes.Equal(randomBytes, bin), "bytes.Equal returned false")
		cancel()
		// first error encountered is remembered.
		n, err := cancellable.Read(buf)
		testhelper.AssertEq(t, 0, n)
		testhelper.AssertErrorsIs(t, err, io.EOF)
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancellable := NewCancellable(ctx, bytes.NewReader(randomBytes))
		_, err := cancellable.Read(buf)
		testhelper.AssertErrorsIs(t, err, nil)
		cancel()
		for i := 0; i < 5; i++ {
			_, err = cancellable.Read(buf)
			testhelper.AssertErrorsIs(t, err, ctx.Err())
		}
	})
}
