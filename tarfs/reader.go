package tarfs

import (
	"archive/tar"
	"fmt"
	"io"
	"math"
	"path"

	"github.com/ngicks/go-fsys-helper/stream"
)

type countingReader struct {
	R     *io.SectionReader
	Count int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.R.Read(p)
	r.Count += n
	return n, err
}

func (r *countingReader) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = r.R.ReadAt(p, off)
	return
}

func (r *countingReader) Seek(offset int64, whence int) (int64, error) {
	n, err := r.R.Seek(offset, whence)
	if err == nil {
		r.Count = int(n)
	}
	return n, err
}

func New(r io.ReaderAt) (*Fs, error) {
	fsys := &Fs{
		headers: make(map[string]*header),
	}

	countingR := &countingReader{R: io.NewSectionReader(r, 0, math.MaxInt-1)}
	tr := tar.NewReader(countingR)

	var (
		prev *header
		blk  block
	)
	for i := 0; ; i++ {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return nil, fmt.Errorf("read tar archive: %w", err)
			}
		}

		headerEnd := countingR.Count

		_, err = io.Copy(io.Discard, tr)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("discarding tar reader: %w", err)
		}
		bodyEnd := countingR.Count

		hh := &header{h: h, headerEnd: headerEnd, bodyStart: headerEnd, bodyEnd: bodyEnd}
		if prev != nil {
			for i := 1; ; i++ {
				if hh.bodyStart-(i*blockSize) < prev.bodyEnd {
					hh.headerStart = hh.bodyStart - ((i - 1) * blockSize)
					break
				}
			}
		}
		hh.holes, _ = reconstructSparse(r, hh, &blk)

		fsys.headers[path.Clean(h.Name)] = hh
		prev = hh
	}
	return fsys, nil
}

type reader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

func makeReader(ra io.ReaderAt, h *header) reader {
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

func reconstructSparse(r io.ReaderAt, hdr *header, blk *block) (sparseHoles, error) {
	if hdr.h.Typeflag == tar.TypeXGlobalHeader {
		return nil, nil
	}

	sr := io.NewSectionReader(r, int64(hdr.headerStart), int64(hdr.headerEnd)-int64(hdr.headerStart))

	for {
		n, err := io.ReadFull(sr, blk[:])
		if (err != nil && err != io.EOF) || n == 0 {
			return nil, err
		}
		switch flag := blk.toV7().typeFlag()[0]; flag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			_, _ = sr.Seek(blockSize, io.SeekCurrent) // read ahead, align to block size
			continue
		case tar.TypeGNULongName, tar.TypeGNULongLink:
			_, _ = sr.Seek(blockSize, io.SeekCurrent)
			continue
		default:
			return handleSparseFile(sr, hdr, blk)
		}
	}
}

func handleSparseFile(sr io.Reader, hdr *header, rawHdr *block) (sparseHoles, error) {
	var spd sparseDatas
	var err error
	if hdr.h.Typeflag == tar.TypeGNUSparse {
		spd, err = readOldGNUSparseMap(sr, rawHdr)
	} else {
		spd, err = readGNUSparsePAXHeaders(sr, hdr)
	}

	if err == nil && spd != nil {
		return invertSparseEntries(spd, hdr.h.Size), nil
	}

	return nil, err
}

func readOldGNUSparseMap(sr io.Reader, blk *block) (sparseDatas, error) {
	var p parser
	s := blk.toGNU().sparse()
	spd := make(sparseDatas, 0, s.maxEntries())
	for {
		for i := 0; i < s.maxEntries(); i++ {
			// This termination condition is identical to GNU and BSD tar.
			if s.entry(i).offset()[0] == 0x00 {
				break // Don't return, need to process extended headers (even if empty)
			}
			offset := p.parseNumeric(s.entry(i).offset())
			length := p.parseNumeric(s.entry(i).length())
			if p.err != nil {
				return nil, p.err
			}
			spd = append(spd, sparseEntry{Offset: offset, Length: length})
		}

		if s.isExtended()[0] > 0 {
			// There are more entries. Read an extension header and parse its entries.
			if _, err := mustReadFull(sr, blk[:]); err != nil {
				return nil, err
			}
			s = blk.toSparse()
			continue
		}
		return spd, nil // Done
	}
}

func readGNUSparsePAXHeaders(sr io.Reader, hdr *header) (sparseDatas, error) {
	// Identify the version of GNU headers.
	var is1x0 bool
	major, minor := hdr.h.PAXRecords[paxGNUSparseMajor], hdr.h.PAXRecords[paxGNUSparseMinor]
	switch {
	case major == "0" && (minor == "0" || minor == "1"):
		is1x0 = false
	case major == "1" && minor == "0":
		is1x0 = true
	case major != "" || minor != "":
		return nil, nil // Unknown GNU sparse PAX version
	case hdr.h.PAXRecords[paxGNUSparseMap] != "":
		is1x0 = false // 0.0 and 0.1 did not have explicit version records, so guess
	default:
		return nil, nil // Not a PAX format GNU sparse file.
	}

	// Read the sparse map according to the appropriate format.
	if is1x0 {
		return readGNUSparseMap1x0(sr)
	}
	return readGNUSparseMap0x1(hdr.h.PAXRecords)
}
