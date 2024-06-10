package stream

import (
	"bytes"
	"crypto/rand"
	"io"
	"sync/atomic"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

var (
	randomBytes      []byte
	randomBytes32KiB []byte
)

func init() {
	var buf bytes.Buffer
	// Too large buffer size causes OOM Kill.
	// Fuzzing uses num of cpu as worker limit.
	// Say you have 24 logical CPU cores,
	// fuzzing will use 24 workers.
	// So it'll allocate bufSize * 24 bytes.
	// num of core may increase over time.
	const bufSize = (33 * 1024) - 19
	_, err := io.CopyN(&buf, rand.Reader, bufSize)
	if err != nil {
		panic(err)
	}
	randomBytes = buf.Bytes()

	var buf2 bytes.Buffer
	const bufSize2 = 32 * 1024
	_, err = io.CopyN(&buf2, rand.Reader, bufSize2)
	if err != nil {
		panic(err)
	}
	randomBytes32KiB = buf2.Bytes()
}

type readReaderAt interface {
	io.Reader
	io.ReaderAt
}

type closable[T readReaderAt] struct {
	R      T
	Closed atomic.Bool
}

func (c *closable[T]) ReadAt(p []byte, off int64) (int, error) {
	return c.R.ReadAt(p, off)
}

func (c *closable[T]) Read(p []byte) (int, error) {
	return c.R.Read(p)
}

func (c *closable[T]) Close() error {
	c.Closed.Store(true)
	return nil
}

func prepareSplittedReader(b []byte, lens []int) []*closable[*bytes.Reader] {
	reader := bytes.NewReader(b)
	var splitted []*closable[*bytes.Reader]
	for i := 0; ; i++ {
		buf := make([]byte, lens[i%len(lens)])
		n, _ := io.ReadAtLeast(reader, buf, 1)
		if n <= 0 {
			break
		}

		splitted = append(splitted, &closable[*bytes.Reader]{R: bytes.NewReader(buf[:n])})
	}
	return splitted
}

// eofReaderAt basically identical to bytes.Reader
// but it returns n, io.EOF if it has read until EOF.
type eofReaderAt struct {
	r *bytes.Reader
}

func (r *eofReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = r.r.ReadAt(p, off)
	if err == nil && int64(r.r.Len()) == off+int64(n) {
		err = io.EOF
	}
	return n, err
}

func prepareSizedReader(b []byte, lens []int, useEofReaderAt bool) []SizedReaderAt {
	reader := bytes.NewReader(b)
	var sizedReaders []SizedReaderAt
	for i := 0; ; i++ {
		buf := make([]byte, lens[i%len(lens)])
		n, _ := io.ReadAtLeast(reader, buf, 1)
		if n <= 0 {
			break
		}

		var readerAt io.ReaderAt = bytes.NewReader(buf[:n])
		if useEofReaderAt {
			readerAt = &eofReaderAt{bytes.NewReader(buf[:n])}
		}
		sizedReaders = append(sizedReaders, SizedReaderAt{
			R:    readerAt,
			Size: int64(n),
		})
	}
	return sizedReaders
}

type onlyWrite struct {
	w io.Writer
}

func (w onlyWrite) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

type onlyRead struct {
	r io.Reader
}

func (r onlyRead) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func useEofReaderAtTestCaseName(b bool) string {
	if b {
		return "use_eofReaderAt"
	}
	return "use_bytesReader"
}

func TestMultiReadCloser(t *testing.T) {
	readers := prepareSplittedReader(randomBytes, []int{1024})
	r := NewMultiReadCloser(readers...)
	for _, reader := range readers {
		testhelper.AssertTrue(t, !reader.Closed.Load(), "Closed is true")
	}
	bin, err := io.ReadAll(r)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertTrue(t, bytes.Equal(randomBytes, bin), "bytes.Equal returned false")
	err = r.Close()
	testhelper.AssertErrorsIs(t, err, nil)
	for _, reader := range readers {
		testhelper.AssertTrue(t, reader.Closed.Load(), "Closed is false")
	}
}
