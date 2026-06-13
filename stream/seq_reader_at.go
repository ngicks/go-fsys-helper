package stream

import (
	"errors"
	"fmt"
	"io"
	"sync"
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
	open func(off int64) (io.ReadCloser, error)
	size int64

	// mu guards the mutable stream state (current, off) so that ReadAt honors
	// the io.ReaderAt contract, which permits concurrent calls. On the
	// sequential hot path the lock is always uncontended.
	mu sync.Mutex
	// current is the kept-open stream; nil when no stream is open.
	// Invariant: off is meaningful only while current != nil. Whenever current
	// is set to nil, off is reset to 0 so a stale offset cannot be mistaken for
	// a live stream position.
	current io.ReadCloser
	off     int64 // logical offset at which current is positioned
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
// Safe for concurrent use. ReadAt and Close serialize on an internal mutex, so
// the returned value honors the [io.ReaderAt] contract (which permits
// concurrent ReadAt calls) and may be composed as a segment of
// [NewMultiReadAtSeekCloser] without making that composite's ReadAt racy. The
// lock is uncontended on the sequential hot path. Note that concurrent ReadAt
// calls at disjoint offsets still defeat the single-open optimization: every
// non-contiguous offset reopens the stream, so concurrency trades open count
// for safety.
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
// It is safe to call concurrently with ReadAt.
func (r *seqReaderAt) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	r.off = 0
	return err
}

// ReadAt implements [io.ReaderAt]. It is safe for concurrent use.
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

	r.mu.Lock()
	defer r.mu.Unlock()

	// Reopen if the requested offset is not the current stream position.
	if r.current == nil || off != r.off {
		if r.current != nil {
			if err := r.current.Close(); err != nil {
				r.current = nil
				r.off = 0
				return 0, fmt.Errorf(
					"NewSeqReaderAt ReadAt: closing stale stream before reopen at offset %d: %w",
					off, err,
				)
			}
			r.current = nil
			r.off = 0
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
				r.off = 0
				return total, io.EOF
			}
			// Propagate non-EOF errors; stream may be unusable.
			_ = r.current.Close()
			r.current = nil
			r.off = 0
			return total, err
		}
	}

	if maxExceeded {
		return total, io.EOF
	}
	return total, nil
}
