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

func makeReader(ra io.ReaderAt, h *Section) seekReadReaderAt {
	if h.holes == nil {
		return io.NewSectionReader(ra, h.BodyStart(), h.BodyEnd()-h.BodyStart())
	}

	var readers []stream.SizedReaderAt

	var cur, size int
	for i, current := range h.holes {
		var prev sparseEntry
		if i > 0 {
			prev = h.holes[i-1]
		}

		space := current.Offset - (prev.Offset + prev.Length)
		if space != 0 { // not first one?
			sr := io.NewSectionReader(ra, h.BodyStart()+int64(cur), space)
			cur += int(space)
			readers = append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
			size += int(sr.Size())
		}
		sr := io.NewSectionReader(stream.NewByteRepeater(0), 0, current.Length)
		readers = append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
		size += int(current.Length)
	}

	if int(h.h.Size) > size {
		sr := io.NewSectionReader(stream.NewByteRepeater(0), 0, h.h.Size-int64(size))
		readers = append(readers, stream.SizedReaderAt{R: sr, Size: sr.Size()})
	}

	return stream.NewMultiReadAtSeekCloser(readers)
}
