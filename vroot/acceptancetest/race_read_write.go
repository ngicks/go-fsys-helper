package acceptancetest

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// TestRaceReadWrite exercises concurrent Read and Write against an open file.
//
// The point is to exercise the implementation under -race; correctness of the
// observed bytes is not asserted (the spec does not require any particular
// interleaving of concurrent reads and writes). The test passes if no race is
// reported by the runtime and no method returns a panic-level error.
func TestRaceReadWrite[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipWriteAt || s.Option.SkipReadAt {
		t.Skip("SkipReadAt or SkipWriteAt is set; positional R/W needed for safe race test")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	// Pre-populate a fixed-size file so reader offsets never go past EOF.
	c.SetupLines(`shared.txt: "0123456789ABCDEF0123456789ABCDEF"`)

	f := c.OpenFile("shared.txt", openFlagReadWrite(), 0)
	defer func() { _ = f.Close() }()

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	var reads, writes atomic.Int64
	g, ctx := errgroup.WithContext(ctx)

	for range 4 {
		g.Go(func() error {
			buf := make([]byte, 4)
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				if _, err := f.ReadAt(buf, 0); err != nil && err != io.EOF {
					if errors.Is(err, vroot.ErrOpNotSupported) {
						return nil
					}
					return err
				}
				reads.Add(1)
			}
		})
	}

	for range 4 {
		g.Go(func() error {
			buf := []byte("WXYZ")
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				if _, err := f.WriteAt(buf, 0); err != nil {
					if errors.Is(err, vroot.ErrOpNotSupported) {
						return nil
					}
					return err
				}
				writes.Add(1)
			}
		})
	}

	if err := g.Wait(); err != nil {
		t.Fatalf("race read/write: %v", err)
	}
	t.Logf("race read/write completed: reads=%d writes=%d", reads.Load(), writes.Load())
}
