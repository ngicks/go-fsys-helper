package fileserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/ngicks/go-fsys-helper/stream"
)

// chunkCount returns the number of chunks required to hold totalSize bytes
// with the given chunkSize. Zero-size objects use exactly one chunk so that
// existence probing via Stat remains uniform (chunk 0 always exists for any
// stored object).
func chunkCount(totalSize, chunkSize int64) int64 {
	if totalSize == 0 {
		return 1 // one empty chunk for zero-size objects
	}
	// Ceiling division: (totalSize + chunkSize - 1) / chunkSize.
	return (totalSize + chunkSize - 1) / chunkSize
}

// expectedChunkSize returns the expected byte count of chunk i.
// All chunks except the last have exactly chunkSize bytes. The last chunk
// holds the remainder, which may be smaller (or zero for a zero-size object).
func expectedChunkSize(i, totalSize, chunkSize int64) int64 {
	n := chunkCount(totalSize, chunkSize)
	if i < n-1 {
		return chunkSize
	}
	// Last chunk.
	if totalSize == 0 {
		return 0
	}
	rem := totalSize % chunkSize
	if rem == 0 {
		return chunkSize
	}
	return rem
}

// ChunkedSource reads a large object that is stored as a sequence of chunk
// objects whose names are provided by a caller-supplied naming function.
//
// Structural compatibility: ChunkedSource has Open(ctx, offset) which matches
// the fsutil.ResumableSource interface without importing fsutil (zero new
// go.mod requirements). A compile-time assertion lives in the consumer
// (oci-image-copy).
//
// Content-addressed chunk names make etags redundant at this layer; the
// OCI-side consumer adapts ChunkedSource to fsutil.ResumableSource and
// supplies the digest as an etag there.
type ChunkedSource struct {
	client    Client
	name      func(i int) string
	totalSize int64
	chunkSize int64
}

// NewChunkedSource constructs a [ChunkedSource].
//
//   - c: the [Client] used to fetch chunks.
//   - name: maps a zero-based chunk index to the object name on the server.
//   - totalSize: total byte count of the logical object.
//   - chunkSize: fixed byte count of every chunk except possibly the last.
func NewChunkedSource(
	c Client, name func(i int) string, totalSize, chunkSize int64,
) *ChunkedSource {
	return &ChunkedSource{
		client:    c,
		name:      name,
		totalSize: totalSize,
		chunkSize: chunkSize,
	}
}

// Open returns an [io.ReadCloser] positioned at the given byte offset within
// the logical object.
//
// offset is mapped to (chunk index, intra-chunk offset). Reading proceeds
// through consecutive chunks lazily: a new GET is issued for each chunk only
// when the reader for the previous chunk is exhausted (one GET per chunk in
// the common sequential case).
//
// The strategy is a lazy sequential concat via [stream.NewSeqReaderAt] +
// [stream.NewMultiReadAtSeekCloser]: segments are only opened on demand as
// the cursor advances, which gives the "one GET per chunk" property while
// still satisfying the [io.ReaderAt] contract required by
// MultiReadAtSeekCloser. ctx governs every underlying Get call.
//
// Errors:
//   - offset < 0 or offset > totalSize: returns an error.
//   - offset == totalSize (or totalSize == 0): returns an empty reader.
func (s *ChunkedSource) Open(
	ctx context.Context, offset int64,
) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, fmt.Errorf(
			"ChunkedSource Open: negative offset %d", offset,
		)
	}
	if offset > s.totalSize {
		return nil, fmt.Errorf(
			"ChunkedSource Open: offset %d exceeds total size %d",
			offset, s.totalSize,
		)
	}
	if offset == s.totalSize {
		// Zero-size object (totalSize==0) or caller positioned at the end:
		// nothing left to read.
		return io.NopCloser(strings.NewReader("")), nil
	}

	nChunks := chunkCount(s.totalSize, s.chunkSize)

	// Build SizedReaderAt segments: one per chunk starting from the chunk
	// that contains offset. In Go 1.22+, loop variables are per-iteration,
	// so i is safely captured by each closure without an explicit copy.
	firstChunk := offset / s.chunkSize
	intraOffset := offset % s.chunkSize

	segments := make([]stream.SizedReaderAt, 0, int(nChunks-firstChunk))
	for i := firstChunk; i < nChunks; i++ {
		sz := expectedChunkSize(i, s.totalSize, s.chunkSize)
		ra := stream.NewSeqReaderAt(func(off int64) (io.ReadCloser, error) {
			// off here is the intra-chunk offset at which we resume.
			rc, _, err := s.client.Get(ctx, s.name(int(i)), off)
			return rc, err
		}, sz)
		segments = append(segments, stream.SizedReaderAt{R: ra, Size: sz})
	}

	multi := stream.NewMultiReadAtSeekCloser(segments)

	// Seek past any intra-chunk offset so reads begin at the right byte.
	if intraOffset > 0 {
		if _, err := multi.Seek(intraOffset, io.SeekStart); err != nil {
			_ = multi.Close()
			return nil, fmt.Errorf(
				"ChunkedSource Open: seek to intra-offset: %w", err,
			)
		}
	}

	return multi, nil
}

// ChunkedSink writes a large object as a sequence of chunk objects.
//
// Structural compatibility: ChunkedSink exposes State / Append / Commit which
// matches the fsutil.ResumableSink interface without importing fsutil.
//
// Resume semantics: push resumes at chunk granularity — an interrupted chunk
// is re-uploaded from its beginning, wasting at most chunkSize bytes.
//
// Zero-size objects: chunk 0 always exists (with 0 bytes) so that existence
// probing via Stat is uniform across all object sizes.
type ChunkedSink struct {
	client    Client
	name      func(i int) string
	totalSize int64
	chunkSize int64
}

// NewChunkedSink constructs a [ChunkedSink].
//
//   - c: the [Client] used to upload chunks.
//   - name: maps a zero-based chunk index to the object name on the server.
//   - totalSize: total byte count of the logical object.
//   - chunkSize: fixed byte count of every chunk except possibly the last.
func NewChunkedSink(
	c Client, name func(i int) string, totalSize, chunkSize int64,
) *ChunkedSink {
	return &ChunkedSink{
		client:    c,
		name:      name,
		totalSize: totalSize,
		chunkSize: chunkSize,
	}
}

// State reports the upload progress.
//
// It probes chunks 0, 1, 2, … in order via Stat. The first chunk that is
// absent (Stat wraps [fs.ErrNotExist]) or has the wrong expected size marks
// the end of the complete prefix. The returned offset is always chunk-aligned
// and equals the number of fully uploaded bytes.
//
// complete is true when all expected chunks are present with the expected
// sizes (including the final short chunk, which may be smaller than
// chunkSize).
//
// Zero-size objects: chunk 0 is expected to exist with size 0. If it is
// present, State returns (0, true, nil).
func (s *ChunkedSink) State(
	ctx context.Context,
) (offset int64, complete bool, err error) {
	nChunks := chunkCount(s.totalSize, s.chunkSize)
	for i := range nChunks {
		sz, statErr := s.client.Stat(ctx, s.name(int(i)))
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				return i * s.chunkSize, false, nil
			}
			return 0, false, fmt.Errorf(
				"ChunkedSink State chunk %d: %w", i, statErr,
			)
		}
		want := expectedChunkSize(i, s.totalSize, s.chunkSize)
		if sz != want {
			// Present but wrong size — treat as absent prefix end.
			return i * s.chunkSize, false, nil
		}
	}
	// All chunks present with correct sizes.
	return s.totalSize, true, nil
}

// Append uploads data from r starting at offset.
//
// offset must be chunk-aligned (a multiple of chunkSize) and must equal
// State().offset — uploading to an arbitrary position is not supported.
//
// Data is read from r in chunkSize-sized pieces. Each piece is spooled to a
// temporary file via [os.CreateTemp] so that its exact size is known before
// issuing the PUT (HTTP requires Content-Length). Temporary files are removed
// on all code paths, including errors.
//
// An interrupted upload leaves at most one partial chunk; the next call to
// State will detect it (wrong size) and return the offset of that chunk, so
// the next Append can re-upload from its start.
func (s *ChunkedSink) Append(
	ctx context.Context, offset int64, r io.Reader,
) error {
	if offset%s.chunkSize != 0 {
		return fmt.Errorf(
			"ChunkedSink Append: offset %d is not chunk-aligned"+
				" (chunkSize=%d)",
			offset, s.chunkSize,
		)
	}

	startChunk := offset / s.chunkSize
	nChunks := chunkCount(s.totalSize, s.chunkSize)

	for i := startChunk; i < nChunks; i++ {
		expected := expectedChunkSize(i, s.totalSize, s.chunkSize)

		// Spool up to chunkSize bytes into a temp file.
		tmp, err := os.CreateTemp("", "chunkedsink-*")
		if err != nil {
			return fmt.Errorf(
				"ChunkedSink Append chunk %d: create temp: %w", i, err,
			)
		}
		tmpName := tmp.Name()
		cleanup := func() {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}

		written, err := io.CopyN(tmp, r, expected)
		if err != nil && err != io.EOF {
			cleanup()
			return fmt.Errorf(
				"ChunkedSink Append chunk %d: spool: %w", i, err,
			)
		}
		// For the last chunk a short read (written < expected) is valid only
		// if the source is exhausted. If this is not the last chunk and we
		// got fewer bytes, the reader was truncated.
		if written < expected && i < nChunks-1 {
			cleanup()
			return fmt.Errorf(
				"ChunkedSink Append chunk %d: short read: got %d, want %d",
				i, written, expected,
			)
		}

		// Rewind temp file for PUT.
		if _, seekErr := tmp.Seek(0, io.SeekStart); seekErr != nil {
			cleanup()
			return fmt.Errorf(
				"ChunkedSink Append chunk %d: seek temp: %w", i, seekErr,
			)
		}

		putErr := s.client.Put(ctx, s.name(int(i)), written, tmp)
		cleanup()
		if putErr != nil {
			return fmt.Errorf(
				"ChunkedSink Append chunk %d: put: %w", i, putErr,
			)
		}
	}
	return nil
}

// Commit is a no-op at this layer.
//
// The real commit is the metadata object written by the OCI-side consumer
// (layer 2) as the final step of a push. Writing the metadata last is the
// crash-consistency invariant: a missing or stale metadata means at most
// orphan chunk objects (harmless for content-addressed names), never a
// metadata referencing missing chunks.
func (s *ChunkedSink) Commit(_ context.Context) error {
	return nil
}
