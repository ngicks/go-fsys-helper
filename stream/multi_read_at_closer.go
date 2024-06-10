package stream

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/ngicks/go-fsys-helper/stream/internal/serr"
)

var (
	// ErrOffset reports an invalid offset passed to Seek.
	ErrOffset = errors.New("invalid offset")
	// ErrInvalidSize reports an incorrectly reported size in []SizedReaderAt caused malformed read from readers.
	// It is very likely wrapped in *MultiReadError.
	ErrInvalidSize = errors.New("invalid size")
	// ErrUnknownWhence reports invalid whence.
	ErrUnknownWhence = errors.New("unknown whence")
)

// MultiReadError is returned by calling Read method of the reader returned from NewMultiReadAtSeekCloser.
type MultiReadError struct {
	Index     int   // Index of readers
	ReaderOff int64 // Offset within a reader
	TotalOff  int64 // The accumulated total offset from the head of readers.
	Err       error
	Cause     string // Additional context info for the error.
}

func (e *MultiReadError) Error() string {
	return fmt.Sprintf(
		"MultiReadError: idx = %d, off = %d, err = %v, cause = %s",
		e.Index, e.ReaderOff, e.Err, e.Cause,
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
	accum int64 // starting offset of this reader from head of readers.
}

type ReadAtReadSeekCloser interface {
	io.ReaderAt
	io.ReadSeekCloser
}

var _ ReadAtReadSeekCloser = (*multiReadAtSeekCloser)(nil)

type multiReadAtSeekCloser struct {
	idx        int   // idx of current sizedReaderAt which is pointed by off.
	off        int64 // current offset
	upperLimit int64 // precomputed upper limit
	r          []sizedReaderAt
}

func NewMultiReadAtSeekCloser(readers []SizedReaderAt) ReadAtReadSeekCloser {
	translated := make([]sizedReaderAt, len(readers))
	var accum = int64(0)
	for i, rr := range readers {
		translated[i] = sizedReaderAt{
			SizedReaderAt: rr,
			accum:         accum,
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

	i := search(r.off, r.r[r.idx:])
	rr := r.r[r.idx:][i]

	off := r.off
	readerOff := r.off - rr.accum
	n, err := rr.R.ReadAt(p, readerOff)

	if n > 0 || err == io.EOF {
		r.idx += i // i could be 0.
		r.off += int64(n)
	}

	if err != nil && err != io.EOF {
		return n, &MultiReadError{i, readerOff, off, err, "read error"}
	}

	switch rem := rr.Size - readerOff; {
	case int64(n) > rem:
		return n, &MultiReadError{i, readerOff, off, ErrInvalidSize, "read more"}
	case err == io.EOF && n == 0 && rem > 0:
		return n, &MultiReadError{i, readerOff, off, io.ErrUnexpectedEOF, "read less"}
	case err == io.EOF && len(r.r)-1 > r.idx:
		err = nil
	}

	return n, err
}

func (r *multiReadAtSeekCloser) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, fmt.Errorf("Seek: %w = %d", ErrUnknownWhence, whence)
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.off
	case io.SeekEnd:
		offset += r.upperLimit
	}
	if offset < 0 {
		return 0, fmt.Errorf("Seek: %w: negative", ErrOffset)
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

	i := search(off, r.r)
	if i < 0 {
		return 0, io.EOF
	}

	rr := r.r[i]
	readerOff := off - rr.accum
	n, err = rr.R.ReadAt(p, readerOff)

	if err != nil && err != io.EOF {
		return n, &MultiReadError{i, readerOff, off, err, "read error"}
	}

	switch rem := rr.Size - readerOff; {
	case int64(n) > rem:
		return n, &MultiReadError{i, readerOff, off, ErrInvalidSize, "read more"}
	case err == io.EOF && n == 0 && rem > 0:
		return n, &MultiReadError{i, readerOff, off, io.ErrUnexpectedEOF, "read less"}
	case err == io.EOF && len(r.r)-1 > i:
		err = nil
	}
	return n, err
}

func (r *multiReadAtSeekCloser) Close() error {
	var errs []error
	for _, rr := range r.r {
		if c, ok := rr.R.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}
	return serr.NewMultiErrorChecked(errs)
}

var searchThreshold int = 32

func search(off int64, readers []sizedReaderAt) int {
	if len(readers) > searchThreshold {
		return binarySearch(off, readers)
	}

	// A simple benchmark has shown that slice look up is faster when readers are not big enough.
	// The threshold exists between 32 and 64.
	for i, rr := range readers {
		if rr.accum <= off && off < rr.accum+rr.Size {
			return i
		}
	}
	return -1
}

func binarySearch(off int64, readers []sizedReaderAt) int {
	i, found := sort.Find(len(readers), func(i int) int {
		switch {
		case off < readers[i].accum:
			return -1
		case readers[i].accum <= off && off < readers[i].accum+readers[i].Size:
			return 0
		default: // r.accum+r.Size <= off:
			return 1
		}
	})
	if !found {
		return -1
	}
	return i
}
