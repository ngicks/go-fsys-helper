package stream

import (
	"bytes"
	"errors"
	"io"
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
