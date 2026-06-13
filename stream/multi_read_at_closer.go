package stream

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"syscall"
)

var (
	// ErrInvalidSize reports an incorrectly reported size in []SizedReaderAt caused malformed read
	// from readers.
	// It is very likely wrapped in *MultiReadError.
	ErrInvalidSize = errors.New("invalid size")
)

// MultiReadError is a detailed error that describes an error state of Read or ReadAt.
type MultiReadError struct {
	// Index of reader that returned the error.
	Index int
	// Offset within the reader at which the error is happened.
	ReaderOff int64
	// The virtual offset within multiReadAtSeekCloser at which the error is happened.
	TotalOff int64
	// Length of buffer with which Read is called.
	BufLen int
	// An internal error.
	// It may be one of an error the reader returned or ErrInvalidSize, or io.ErrUnexpectedEOF.
	// It is ErrInvalidSize when the reader read more than reported in SizedReaderAt.
	// Or is io.ErrUnexpectedEOF when the reader read less than that.
	Err error
	// Additional context info for the error.
	Cause string
}

func (e *MultiReadError) Error() string {
	return fmt.Sprintf(
		"MultiReadError: idx = %d, off = %d, totalOff = %d, bufLen = %d, err = %v, cause = %s",
		e.Index, e.ReaderOff, e.TotalOff, e.BufLen, e.Err, e.Cause,
	)
}

func (e *MultiReadError) Unwrap() error {
	return e.Err
}

type SizedReaderAt struct {
	R    io.ReaderAt
	Size int64
}

type FileLike interface {
	Stat() (fs.FileInfo, error)
	io.ReaderAt
}

// SizedReadersFromFileLike constructs []SizedReaderAt from file like objects.
// For example, *os.File and afero.File implement FileLike.
func SizedReadersFromFileLike[T FileLike](files []T) ([]SizedReaderAt, error) {
	sizedReaders := make([]SizedReaderAt, len(files))
	for i, f := range files {
		s, err := f.Stat()
		if err != nil {
			return nil, err
		}
		sizedReaders[i] = SizedReaderAt{
			R:    f,
			Size: s.Size(),
		}
	}
	return sizedReaders, nil
}

type ReadAtSizer interface {
	io.ReaderAt
	Size() int64
}

// SizedReadersFromReadAtSizer constructs []SizedReaderAt from ReaderAt with Size method.
// For example, *io.SectionReader implements ReadAtSizer.
func SizedReadersFromReadAtSizer[T ReadAtSizer](readers []T) []SizedReaderAt {
	sizedReaders := make([]SizedReaderAt, len(readers))
	for i, r := range readers {
		sizedReaders[i] = SizedReaderAt{
			R:    r,
			Size: r.Size(),
		}
	}
	return sizedReaders
}

type sizedReaderAt struct {
	SizedReaderAt
	// starting offset of this reader from head of all readers.
	// This will come handy when searching for reader from off,
	// especially useful when Seek or ReadAt is called.
	headOff int64
}

type ReadAtReadSeekCloser interface {
	io.ReaderAt
	io.ReadSeekCloser
}

var _ ReadAtReadSeekCloser = (*multiReadAtSeekCloser)(nil)

type multiReadAtSeekCloser struct {
	idx        int   // idx of current sizedReaderAt which is pointed by off.
	off        int64 // current offset
	upperLimit int64 // precomputed upper limit of offset.
	r          []sizedReaderAt
}

// NewMultiReadAtSeekCloser virtually concatenates readers into a single reader.
// Unlike io.MultiReader it implements io.ReaderAt.
//
// Each [SizedReaderAt.Size] must be non-negative; a negative size is a
// programmer error (it would corrupt the precomputed headOff/upperLimit math)
// and NewMultiReadAtSeekCloser panics. See [NewSeqReaderAt] for the same policy.
//
// Concurrency: the returned value's ReadAt is safe to call concurrently only
// when every segment's own ReadAt is. The composite forwards each ReadAt to the
// segment that owns the offset, so a segment that is not concurrency-safe makes
// the composite's ReadAt racy at that offset. Segments built with
// [NewSeqReaderAt] and [io.SectionReader] are both safe for concurrent ReadAt.
// The Read/Seek path is sequential and is not safe for concurrent use.
func NewMultiReadAtSeekCloser(readers []SizedReaderAt) ReadAtReadSeekCloser {
	translated := make([]sizedReaderAt, len(readers))
	var accum = int64(0)
	for i, rr := range readers {
		if rr.Size < 0 {
			panic(fmt.Sprintf(
				"NewMultiReadAtSeekCloser: SizedReaderAt at index %d has negative Size %d",
				i, rr.Size,
			))
		}
		translated[i] = sizedReaderAt{
			SizedReaderAt: rr,
			headOff:       accum,
		}
		accum += rr.Size
	}
	return &multiReadAtSeekCloser{
		upperLimit: accum,
		r:          translated,
	}
}

func (r *multiReadAtSeekCloser) Read(p []byte) (int, error) {
	if r.off >= r.upperLimit {
		return 0, io.EOF
	}

	// search is performed on the tail r.r[r.idx:], so the returned index is
	// relative; idx is the absolute segment index the read targets.
	i := search(r.off, r.r[r.idx:])
	idx := r.idx + i

	n, isEOF, err := r.readSegment(idx, p, r.off)

	// Advance the cursor whenever bytes were produced or the segment was fully
	// consumed (EOF). This uses the absolute index so state stays consistent
	// even when zero-length segments made search skip forward (i > 0).
	if n > 0 || isEOF {
		r.idx = idx
		r.off += int64(n)
	}

	return n, err
}

func (r *multiReadAtSeekCloser) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, fmt.Errorf("Seek: %w: %d", syscall.EINVAL, whence)
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.off
	case io.SeekEnd:
		offset += r.upperLimit
	}
	if offset < 0 {
		return 0, fmt.Errorf("Seek: %w: negative", syscall.EINVAL)
	}

	r.off = offset

	if r.off >= r.upperLimit {
		r.idx = len(r.r)
		return r.off, nil
	}

	r.idx = search(r.off, r.r)

	return r.off, nil
}

// ReadAt implements io.ReaderAt.
func (r *multiReadAtSeekCloser) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= r.upperLimit {
		return 0, io.EOF
	}
	maxExceeded := false
	if max := r.upperLimit - off; int64(len(p)) > max {
		maxExceeded = true
		p = p[0:max]
	}
	for {
		nn, err := r.readAt(p, off)
		n += nn
		off += int64(nn)
		if nn == len(p) || err != nil {
			if maxExceeded && err == nil {
				err = io.EOF
			}
			return n, err
		}
		p = p[nn:]
	}
}

// readAt reads from a single ReaderAt at translated offset.
func (r *multiReadAtSeekCloser) readAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= r.upperLimit {
		return 0, io.EOF
	}

	idx := search(off, r.r)
	if idx < 0 {
		return 0, io.EOF
	}

	n, _, err = r.readSegment(idx, p, off)
	return n, err
}

// readSegment performs a single ReadAt against the segment at the absolute
// index idx, where off is the virtual offset within the whole concatenation.
// It is the one place that reads a segment and validates the result; both Read
// and readAt delegate to it so the error classification, size validation, and
// (crucially) the absolute segment index recorded in any MultiReadError stay in
// one implementation.
//
// It returns the number of bytes read, whether the segment reached EOF (so the
// caller may advance its cursor), and the classified error:
//   - a *MultiReadError on a hard read error, a too-large read (ErrInvalidSize),
//     or a too-short read (io.ErrUnexpectedEOF), always carrying the absolute
//     index idx;
//   - nil when the segment hit EOF but a later segment still has data (so the
//     concatenation should continue);
//   - io.EOF when the final segment hit EOF.
func (r *multiReadAtSeekCloser) readSegment(idx int, p []byte, off int64) (n int, isEOF bool, err error) {
	rr := r.r[idx]
	readerOff := off - rr.headOff
	n, err = rr.R.ReadAt(p, readerOff)

	// Classify EOF with errors.Is so a segment returning a wrapped EOF
	// (e.g. fmt.Errorf("...: %w", io.EOF)) is treated as a clean end-of-segment
	// rather than a hard error. Normalize to the canonical io.EOF so a wrapped
	// EOF surfaced from the final segment does not leak to callers (e.g.
	// io.ReadAll) that compare against io.EOF. This cannot mask a short read
	// because the rem-based validation below converts a too-short read into
	// io.ErrUnexpectedEOF.
	isEOF = errors.Is(err, io.EOF)
	if isEOF {
		err = io.EOF
	}

	wrapErr := func(err error, cause string) error {
		return &MultiReadError{idx, readerOff, off, len(p), err, cause}
	}

	if err != nil && !isEOF {
		return n, isEOF, wrapErr(err, "read error")
	}

	switch rem := rr.Size - readerOff; {
	case int64(n) > rem:
		return n, isEOF, wrapErr(ErrInvalidSize, "read more")
	case isEOF && n == 0 && rem > 0:
		return n, isEOF, wrapErr(io.ErrUnexpectedEOF, "read less")
	case isEOF && len(r.r)-1 > idx:
		err = nil
	}
	return n, isEOF, err
}

func (r *multiReadAtSeekCloser) Close() error {
	return closeFanOut(r.r, func(rr sizedReaderAt) (error, bool) {
		// A segment is closed only if its ReaderAt implements io.Closer;
		// otherwise the index is skipped entirely.
		c, ok := rr.R.(io.Closer)
		if !ok {
			return nil, false
		}
		return c.Close(), true
	})
}

var searchThreshold int = 32

func search(off int64, readers []sizedReaderAt) int {
	if len(readers) > searchThreshold {
		return binarySearch(off, readers)
	}

	// A simple benchmark has shown that slice look up is faster when readers are not big enough.
	// The threshold exists between 32 and 64.
	for i, rr := range readers {
		if rr.headOff <= off && off < rr.headOff+rr.Size {
			return i
		}
	}
	return -1
}

func binarySearch(off int64, readers []sizedReaderAt) int {
	i, found := slices.BinarySearchFunc(readers, off, func(r sizedReaderAt, off int64) int {
		switch {
		case off < r.headOff:
			return 1
		case r.headOff <= off && off < r.headOff+r.Size:
			return 0
		default: // r.headOff+r.Size <= off:
			return -1
		}
	})
	if !found {
		return -1
	}
	return i
}
