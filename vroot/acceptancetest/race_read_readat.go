package acceptancetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// TestRaceReadReadAt exercises concurrent Read and ReadAt against a single open
// file handle.
//
// ReadAt's defining guarantee is positional independence: per [io.ReaderAt],
// "ReadAt should not affect nor be affected by the underlying seek offset."
// So while Read (and Seek) churn the shared offset from other goroutines, a
// ReadAt at a fixed offset must keep returning the bytes at that offset. The
// test asserts exactly that, which catches a ReadAt implemented as
// seek-then-read (offset shared with Read). It also runs clean under -race.
//
// This lives in the read-only suite so read-only implementations are covered.
func TestRaceReadReadAt[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipReadAt {
		t.Skip("SkipReadAt is set; ReadAt needed for this race test")
	}

	// Distinct bytes per position so a corrupted offset yields a visible
	// mismatch rather than coincidentally-equal data. The fixture is passed
	// to Make so read-only implementations can materialize it out-of-band
	// instead of writing through the (possibly read-only) fs under test.
	const content = "0123456789ABCDEFGHIJKLMNOPQRSTUV"
	fsys := makeFs(t, s, `shared.txt: "`+content+`"`)
	c := newC(t, fsys)

	f := c.Open("shared.txt")
	defer func() { _ = f.Close() }()

	const fixedOff = 8
	const readLen = 4
	want := content[fixedOff : fixedOff+readLen]

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	var readAts, reads atomic.Int64
	g, ctx := errgroup.WithContext(ctx)

	// ReadAt goroutines: must always observe the bytes at fixedOff regardless
	// of the concurrent Read/Seek churn on the shared offset.
	for range 4 {
		g.Go(func() error {
			buf := make([]byte, readLen)
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				n, err := f.ReadAt(buf, fixedOff)
				if err != nil && err != io.EOF {
					if errors.Is(err, vroot.ErrOpNotSupported) {
						return nil
					}
					return err
				}
				if n == readLen && string(buf) != want {
					return fmt.Errorf("ReadAt(%d) = %q, want %q: concurrent Read corrupted ReadAt's offset", fixedOff, buf, want)
				}
				readAts.Add(1)
			}
		})
	}

	// Read goroutines: advance the shared offset, resetting with Seek at EOF.
	for range 4 {
		g.Go(func() error {
			buf := make([]byte, readLen)
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				_, err := f.Read(buf)
				if errors.Is(err, io.EOF) {
					if s.Option.SkipSeek {
						return nil // cannot rewind without Seek
					}
					if _, err := f.Seek(0, io.SeekStart); err != nil {
						if errors.Is(err, vroot.ErrOpNotSupported) {
							return nil
						}
						return err
					}
					continue
				}
				if err != nil {
					return err
				}
				reads.Add(1)
			}
		})
	}

	if err := g.Wait(); err != nil {
		t.Fatalf("race read/readat: %v", err)
	}
	t.Logf("race read/readat completed: readAts=%d reads=%d", readAts.Load(), reads.Load())
}
