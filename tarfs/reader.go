package tarfs

import (
	"io"

	"github.com/ngicks/go-fsys-helper/stream"
)

type seekReadReaderAt interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

func makeReader(ra io.ReaderAt, h *header) seekReadReaderAt {
	if h.holes == nil {
		return io.NewSectionReader(ra, int64(h.bodyStart), int64(h.bodyEnd)-int64(h.bodyStart))
	}

	var readers []stream.SizedReaderAt

	holes := h.holes
	appendZeroRepater := func(readers []stream.SizedReaderAt, hole sparseEntry) []stream.SizedReaderAt {
		sr := io.NewSectionReader(stream.NewByteRepeater(0), 0, hole.Length)
		return append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
	}

	var cur, size int
	for i, current := range holes {
		var prev sparseEntry
		if i > 0 {
			prev = holes[i-1]
		}

		space := current.Offset - (prev.Offset + prev.Length)
		if space != 0 { // not first one?
			sr := io.NewSectionReader(ra, int64(h.bodyStart)+int64(cur), space)
			cur += int(space)
			readers = append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
			size += int(sr.Size())
		}
		readers = appendZeroRepater(readers, current)
		size += int(current.Length)
	}

	if int(h.h.Size) > size {
		sr := io.NewSectionReader(stream.NewByteRepeater(0), 0, h.h.Size-int64(size))
		readers = append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
	}

	return stream.NewMultiReadAtSeekCloser(readers)
}
