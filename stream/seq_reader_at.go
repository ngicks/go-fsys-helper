package stream

import (
	"errors"
	"fmt"
	"io"
)

// ReadAtSizeCloser combines [io.ReaderAt], a Size method, and [io.Closer].
// It is the type returned by [NewSeqReaderAt] and accepted by helpers such as
// [SizedReadersFromReadAtSizer] (via the [ReadAtSizer] subset).
type ReadAtSizeCloser interface {
	ReadAtSizer
	io.Closer
}

// seqReaderAt is the concrete implementation returned by [NewSeqReaderAt].
type seqReaderAt struct {
	open    func(off int64) (io.ReadCloser, error)
	size    int64
	current io.ReadCloser // kept-open stream; nil when no stream is open
	off     int64         // logical offset at which current is positioned
}

// NewSeqReaderAt returns a [ReadAtSizeCloser] backed by an offset-opener
// (for example a function that issues a ranged HTTP GET starting at off).
//
// Sequential access pattern — one open per segment:
// ReadAt calls with monotonically increasing offsets are served from a single
// kept-open stream without reopening. A backward jump or a discontiguous
// forward jump (i.e. an offset greater than the current stream position)
// closes the current stream and calls open at the new offset.
//
// NOT safe for concurrent use. This matches the sequential Read path of
// [NewMultiReadAtSeekCloser]: the multiReadAtSeekCloser.Read method drives a
// single offset forward and calls ReadAt on each segment once. Its ReadAt
// method, however, is safe to call concurrently by users — callers who need
// to use a SeqReaderAt as a segment inside a MultiReadAtSeekCloser and then
// call ReadAt concurrently on the outer object must not use NewSeqReaderAt for
// those segments; instead they should use a fully seekable source (e.g.
// [io.SectionReader] over a local file).
//
// Errors from open are wrapped with the offset at which the open was
// attempted, e.g.: "NewSeqReaderAt open at offset 1024: <underlying error>".
//
// EOF semantics follow the [io.ReaderAt] contract: a read starting at or past
// size returns (0, io.EOF); a read that fills p before reaching size returns
// (n, nil); a read that hits the end of the data returns (n, io.EOF).
//
// size must be non-negative; a negative size is a programmer error and
// NewSeqReaderAt panics.
func NewSeqReaderAt(
	open func(off int64) (io.ReadCloser, error), size int64,
) ReadAtSizeCloser {
	if size < 0 {
		panic(fmt.Sprintf("NewSeqReaderAt: negative size %d", size))
	}
	return &seqReaderAt{open: open, size: size}
}

// Size implements [ReadAtSizer].
func (r *seqReaderAt) Size() int64 { return r.size }

// Close closes the currently open stream, if any.
func (r *seqReaderAt) Close() error {
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}

// ReadAt implements [io.ReaderAt].
//
// Reads starting at or past r.size return (0, io.EOF) immediately.
// Reads that would extend past r.size are clamped to the available data and
// return (n, io.EOF) after reading all available bytes.
func (r *seqReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("NewSeqReaderAt ReadAt: negative offset %d", off)
	}
	if off >= r.size {
		return 0, io.EOF
	}

	// Clamp p to avoid reading past size.
	maxExceeded := false
	if avail := r.size - off; int64(len(p)) > avail {
		p = p[:avail]
		maxExceeded = true
	}

	// Reopen if the requested offset is not the current stream position.
	if r.current == nil || off != r.off {
		if r.current != nil {
			if err := r.current.Close(); err != nil {
				r.current = nil
				return 0, err
			}
			r.current = nil
		}
		rc, err := r.open(off)
		if err != nil {
			return 0, fmt.Errorf(
				"NewSeqReaderAt open at offset %d: %w", off, err,
			)
		}
		r.current = rc
		r.off = off
	}

	// Read, advancing r.off as we go.
	var total int
	for len(p) > 0 {
		n, err := r.current.Read(p)
		total += n
		r.off += int64(n)
		p = p[n:]
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Stream ended — close and discard so next call reopens.
				// Classify with errors.Is so a wrapped EOF from a user-supplied
				// stream is handled, and normalize to the canonical io.EOF.
				_ = r.current.Close()
				r.current = nil
				return total, io.EOF
			}
			// Propagate non-EOF errors; stream may be unusable.
			_ = r.current.Close()
			r.current = nil
			return total, err
		}
	}

	if maxExceeded {
		return total, io.EOF
	}
	return total, nil
}
