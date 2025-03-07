package tarfs

import (
	"archive/tar"
	"fmt"
	"io"
	"math"
	"path"
)

type header struct {
	h                      *tar.Header
	headerStart, headerEnd int
	bodyStart, bodyEnd     int
	holes                  sparseHoles
}

func collectHeaders(r io.ReaderAt) (map[string]*header, error) {
	// first collect entries in the map
	// Tar archives may have duplicate entry for same name for incremental update, etc.
	headers := make(map[string]*header)

	countingR := &countingReader{R: io.NewSectionReader(r, 0, math.MaxInt64-1)}
	tr := tar.NewReader(countingR)

	var (
		prev *header
		blk  block
	)
	for {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return nil, fmt.Errorf("read tar archive: %w", err)
			}
		}

		headerEnd := countingR.Count

		hh := &header{h: h, headerEnd: headerEnd, bodyStart: headerEnd}
		if prev != nil {
			// bodyEnd padded to 512 bytes block boundary
			hh.headerStart = prev.bodyEnd + (-prev.bodyEnd)&(blockSize-1)
		}

		hh.holes, _ = reconstructSparse(r, hh, &blk)

		switch hh.h.Typeflag {
		case tar.TypeLink, tar.TypeSymlink, tar.TypeChar, tar.TypeBlock, tar.TypeDir, tar.TypeFifo,
			tar.TypeCont, tar.TypeXHeader, tar.TypeXGlobalHeader,
			tar.TypeGNULongName, tar.TypeGNULongLink:
			// They have size for name.
			hh.bodyEnd = hh.bodyStart
		default:
			// Not totally sure but in testdata tars there's typeflag value not defined in archive/tar
			// nor there https://www.gnu.org/software/tar/manual/html_node/Standard.html
			hh.bodyEnd = hh.bodyStart + int(hh.h.Size)
			if hh.holes != nil {
				// reverse-caluculating size
				// I dunno how many tar files out wilds have sparse in them.
				var holeSize int
				for _, hole := range hh.holes {
					holeSize += int(hole.Length)
				}
				hh.bodyEnd = hh.bodyStart + int(hh.h.Size) - holeSize
			}
		}

		headers[path.Clean(h.Name)] = hh
		prev = hh
	}

	return headers, nil
}

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

// Seek will be used by tar.Reader.Next.
func (r *countingReader) Seek(offset int64, whence int) (int64, error) {
	n, err := r.R.Seek(offset, whence)
	if err == nil {
		r.Count = int(n)
	}
	return n, err
}

func reconstructSparse(r io.ReaderAt, hdr *header, blk *block) (sparseHoles, error) {
	if hdr.h.Typeflag == tar.TypeXGlobalHeader {
		return nil, nil
	}

	sr := io.NewSectionReader(r, int64(hdr.headerStart), int64(hdr.headerEnd)-int64(hdr.headerStart))

	var p parser
	for {
		n, err := io.ReadFull(sr, blk[:])
		if (err != nil && err != io.EOF) || n == 0 {
			return nil, err
		}
		switch flag := blk.toV7().typeFlag()[0]; flag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			size := p.parseNumeric(blk.toV7().size())
			size += (-size) & (blockSize - 1)
			_, _ = sr.Seek(size, io.SeekCurrent) // read ahead, align to block size
			continue
		case tar.TypeGNULongName, tar.TypeGNULongLink:
			size := p.parseNumeric(blk.toV7().size())
			size += (-size) & (blockSize - 1)
			_, _ = sr.Seek(size, io.SeekCurrent)
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
