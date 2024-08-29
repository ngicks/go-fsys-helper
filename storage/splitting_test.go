package storage

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"fmt"
	"io"
	"testing"

	"gotest.tools/v3/assert"
)

var randomBytes []byte

func init() {
	var buf bytes.Buffer
	_, err := io.CopyN(&buf, rand.Reader, 31000)
	if err != nil {
		panic(err)
	}
	randomBytes = buf.Bytes()
}

func TestSplitter(t *testing.T) {
	for _, size := range []uint{
		13,
		7 * 1024,
		41 * 1024,
		10 * 1024,
		31 * 1024,
		(31 * 1024) + 1,
		32 * 1024,
	} {
		for _, builder := range [](func(b []byte) (io.Reader, string)){
			func(b []byte) (io.Reader, string) { return bytes.NewReader(b), "*bytes.Reader" },
			func(b []byte) (io.Reader, string) { return &eofReader{buf: b}, "*eofReader" },
		} {
			b, name := builder(randomBytes)
			t.Run(fmt.Sprintf("%s,%d", name, size), func(t *testing.T) {
				splitter := SplitReader(b, size)

				assert.Assert(t, splitter.Size() == int(size))

				var buf bytes.Buffer
				for {
					r, ok := splitter.Next()
					if !ok {
						assert.Assert(t, r == nil)
						break
					}
					n, err := io.Copy(&buf, r)
					if n > 0 {
						assert.Assert(t, n <= int64(size))
					}
					assert.NilError(t, err)
				}

				r, ok := splitter.Next()
				assert.Assert(t, !ok)
				assert.Assert(t, r == nil)
				assert.Assert(t, bytes.Equal(buf.Bytes(), randomBytes))
			})
		}
	}
}

type eofReader struct {
	i   int
	buf []byte
}

func (r *eofReader) Read(p []byte) (int, error) {
	if r.i >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.i:])
	r.i += n
	if r.i == len(r.buf) {
		return n, io.EOF
	}
	return n, nil
}
