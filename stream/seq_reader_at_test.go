package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

// TestSeqReaderAt_monotonic verifies that monotonically increasing ReadAt
// offsets reuse a single open stream (open is called only once per run).
func TestSeqReaderAt_monotonic(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	openCount := 0
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		openCount++
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))

	buf := make([]byte, 4)

	// First read at offset 0 — opens stream #1.
	n, err := ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[0:4], buf[:n]), "data mismatch offset 0")
	testhelper.AssertEq(t, 1, openCount)

	// Read at offset 4 — same stream, no reopen.
	n, err = ra.ReadAt(buf, 4)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[4:8], buf[:n]), "data mismatch offset 4")
	testhelper.AssertEq(t, 1, openCount)

	// Read at offset 8 — monotonic, still same stream.
	n, err = ra.ReadAt(buf, 8)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[8:12], buf[:n]), "data mismatch offset 8")
	testhelper.AssertEq(t, 1, openCount)

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_backward verifies that a backward seek closes the current
// stream and reopens at the new offset.
func TestSeqReaderAt_backward(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	openCount := 0
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		openCount++
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))

	buf := make([]byte, 4)

	// Read at offset 10 — opens stream #1.
	_, _ = ra.ReadAt(buf, 10)
	testhelper.AssertEq(t, 1, openCount)

	// Read at offset 2 — backward, must reopen.
	n, err := ra.ReadAt(buf, 2)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[2:6], buf[:n]), "data mismatch after backward seek")
	testhelper.AssertEq(t, 2, openCount)

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_jump verifies that a discontiguous forward jump (skipping
// bytes) also causes a reopen.
func TestSeqReaderAt_jump(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	openCount := 0
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		openCount++
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))

	buf := make([]byte, 4)

	// Read at 0 consumes bytes 0..3; stream is at position 4.
	_, _ = ra.ReadAt(buf, 0)
	testhelper.AssertEq(t, 1, openCount)

	// Jump forward to 10 — discontiguous, must reopen.
	n, err := ra.ReadAt(buf, 10)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[10:14], buf[:n]), "data mismatch after jump")
	testhelper.AssertEq(t, 2, openCount)

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_EOF verifies that a read starting at or past size returns
// (0, io.EOF) immediately.
func TestSeqReaderAt_EOF(t *testing.T) {
	data := []byte("hello")
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))

	buf := make([]byte, 10)

	// Read at exactly size.
	n, err := ra.ReadAt(buf, int64(len(data)))
	testhelper.AssertEq(t, 0, n)
	testhelper.AssertErrorsIs(t, err, io.EOF)

	// Read past size.
	n, err = ra.ReadAt(buf, int64(len(data))+100)
	testhelper.AssertEq(t, 0, n)
	testhelper.AssertErrorsIs(t, err, io.EOF)

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_short_segment verifies that a read whose buffer extends
// past size is clamped and returns io.EOF after filling the available bytes.
func TestSeqReaderAt_short_segment(t *testing.T) {
	data := []byte("hello")
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))

	// Buffer larger than the data — should read all 5 bytes and return EOF.
	buf := make([]byte, 20)
	n, err := ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, io.EOF)
	testhelper.AssertEq(t, len(data), n)
	testhelper.AssertTrue(t, bytes.Equal(data, buf[:n]), "data mismatch on short segment")

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_open_error verifies that errors from the open function are
// propagated and include the offset context.
func TestSeqReaderAt_open_error(t *testing.T) {
	sentinel := errors.New("dial refused")
	ra := NewSeqReaderAt(func(_ int64) (io.ReadCloser, error) {
		return nil, sentinel
	}, 100)

	buf := make([]byte, 10)
	_, err := ra.ReadAt(buf, 5)
	testhelper.AssertErrorsIs(t, err, sentinel)
	// Error message must contain the offset.
	testhelper.AssertErrorContains(t, err, "5")
}

// TestSeqReaderAt_compose_multi verifies that NewSeqReaderAt composes
// correctly with NewMultiReadAtSeekCloser: three sequential segments backed by
// separate openers concatenate into the full data.
func TestSeqReaderAt_compose_multi(t *testing.T) {
	// Three chunks of 10 bytes each.
	chunks := [][]byte{
		bytes.Repeat([]byte{0x01}, 10),
		bytes.Repeat([]byte{0x02}, 10),
		bytes.Repeat([]byte{0x03}, 10),
	}
	full := append(append(append([]byte{}, chunks[0]...), chunks[1]...), chunks[2]...)

	openCounts := make([]int, len(chunks))
	segs := make([]SizedReaderAt, len(chunks))
	for i, ch := range chunks {
		ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
			openCounts[i]++
			return io.NopCloser(bytes.NewReader(ch[off:])), nil
		}, int64(len(ch)))
		segs[i] = SizedReaderAt{R: ra, Size: int64(len(ch))}
	}

	multi := NewMultiReadAtSeekCloser(segs)
	got, err := io.ReadAll(multi)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertTrue(t, bytes.Equal(full, got), "compose_multi: data mismatch")
	testhelper.AssertErrorsIs(t, multi.Close(), nil)
}

// wrappedEofReadCloser wraps a *bytes.Reader and returns a wrapped io.EOF at
// the end of the stream, exercising the errors.Is-based EOF classification in
// seqReaderAt.ReadAt.
type wrappedEofReadCloser struct {
	r *bytes.Reader
}

func (rc *wrappedEofReadCloser) Read(p []byte) (int, error) {
	n, err := rc.r.Read(p)
	if errors.Is(err, io.EOF) {
		err = errors.Join(errors.New("stream closed"), io.EOF)
	}
	return n, err
}

func (rc *wrappedEofReadCloser) Close() error { return nil }

// TestSeqReaderAt_wrapped_eof verifies that a stream returning a wrapped io.EOF
// is handled as a clean end-of-stream: ReadAt clamped past size returns the
// available bytes and a canonical io.EOF, not the wrapped error.
func TestSeqReaderAt_wrapped_eof(t *testing.T) {
	data := []byte("hello")
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		return &wrappedEofReadCloser{bytes.NewReader(data[off:])}, nil
	}, int64(len(data)))

	buf := make([]byte, 20) // larger than data to force EOF on Read
	n, err := ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, io.EOF)
	testhelper.AssertEq(t, len(data), n)
	testhelper.AssertTrue(t, bytes.Equal(data, buf[:n]), "wrapped_eof: data mismatch")

	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// TestSeqReaderAt_concurrent_ReadAt fires many concurrent ReadAt calls at
// disjoint offsets and verifies every read returns the correct bytes. Before
// the mutex was added this raced on r.current/r.off (run with -race to detect).
// The opener returns a fresh reader over the shared immutable data on every
// call, so it is itself safe for concurrent use.
func TestSeqReaderAt_concurrent_ReadAt(t *testing.T) {
	data := randomBytes32KiB
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data[off:])), nil
	}, int64(len(data)))
	defer func() { testhelper.AssertErrorsIs(t, ra.Close(), nil) }()

	const (
		chunk   = 256
		workers = 32
	)
	nChunks := len(data) / chunk

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			buf := make([]byte, chunk)
			// Each worker sweeps all chunks but starts at a different one so
			// the offsets interleave across goroutines.
			for k := 0; k < nChunks; k++ {
				idx := (seed + k) % nChunks
				off := int64(idx * chunk)
				n, err := ra.ReadAt(buf, off)
				if err != nil && !errors.Is(err, io.EOF) {
					errCh <- fmt.Errorf("worker %d off %d: %w", seed, off, err)
					return
				}
				if n != chunk {
					errCh <- fmt.Errorf("worker %d off %d: short read n=%d", seed, off, n)
					return
				}
				if !bytes.Equal(data[off:off+chunk], buf) {
					errCh <- fmt.Errorf("worker %d off %d: data mismatch", seed, off)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

// TestSeqReaderAt_stale_close_error verifies that a failure closing the stale
// stream before a reopen is wrapped with context (and the offset), and that the
// reader recovers (off reset to 0) so a subsequent read succeeds.
func TestSeqReaderAt_stale_close_error(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	closeErr := errors.New("close failed")
	failClose := true
	ra := NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
		return &errCloseReadCloser{
			r: bytes.NewReader(data[off:]),
			closeErr: func() error {
				if failClose {
					return closeErr
				}
				return nil
			},
		}, nil
	}, int64(len(data)))

	buf := make([]byte, 4)
	// Open stream #1 at offset 0; stream advances to position 4.
	_, err := ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, nil)

	// Backward jump to 0 forces a reopen; closing the stale stream fails.
	_, err = ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, closeErr)
	testhelper.AssertErrorContains(t, err, "stale stream")
	testhelper.AssertErrorContains(t, err, "0") // offset context

	// Recovery: subsequent close succeeds now; reader must reopen cleanly.
	failClose = false
	n, err := ra.ReadAt(buf, 0)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 4, n)
	testhelper.AssertTrue(t, bytes.Equal(data[0:4], buf), "recovery: data mismatch")

	failClose = false
	testhelper.AssertErrorsIs(t, ra.Close(), nil)
}

// errCloseReadCloser is an io.ReadCloser whose Close returns the result of
// closeErr() each time it is called.
type errCloseReadCloser struct {
	r        *bytes.Reader
	closeErr func() error
}

func (rc *errCloseReadCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc *errCloseReadCloser) Close() error               { return rc.closeErr() }

// TestSeqReaderAt_negative_size verifies that NewSeqReaderAt panics on a
// negative size (D3: programmer error, no silent clamping).
func TestSeqReaderAt_negative_size(t *testing.T) {
	defer func() {
		r := recover()
		testhelper.AssertNonNilInterface(t, r)
		msg, ok := r.(string)
		testhelper.AssertTrue(t, ok, "recover() is not a string: %#v", r)
		testhelper.AssertTrue(t,
			strings.Contains(msg, "negative size"),
			"panic message %q does not mention negative size", msg,
		)
	}()
	NewSeqReaderAt(func(int64) (io.ReadCloser, error) { return nil, nil }, -1)
}
