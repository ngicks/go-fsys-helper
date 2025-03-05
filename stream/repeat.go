package stream

import (
	"io"
)

var (
	_ io.Reader   = (*ByteRepeater)(nil)
	_ io.ReaderAt = (*ByteRepeater)(nil)
)

// ByteRepeater infinitely reads byte b.
// Use with [io.SectionReader] if the reader should be limited.
type ByteRepeater struct {
	b byte
}

func NewByteRepeater(b byte) *ByteRepeater {
	return &ByteRepeater{b: b}
}

func (r *ByteRepeater) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}
func (r *ByteRepeater) ReadAt(p []byte, off int64) (n int, err error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}
