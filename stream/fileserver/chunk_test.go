package fileserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/fileserver"
	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

// ---- in-memory Client for chunk tests ----

// inMemClient is a simple in-memory [fileserver.Client] that records GETs.
type inMemClient struct {
	objects map[string][]byte
	getLog  []string
}

func newInMemClient() *inMemClient {
	return &inMemClient{objects: make(map[string][]byte)}
}

func (m *inMemClient) Get(
	_ context.Context, name string, offset int64,
) (io.ReadCloser, int64, error) {
	m.getLog = append(m.getLog, name)
	data, ok := m.objects[name]
	if !ok {
		return nil, 0, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	return io.NopCloser(bytes.NewReader(data[offset:])), int64(len(data)), nil
}

func (m *inMemClient) Stat(
	_ context.Context, name string,
) (int64, error) {
	data, ok := m.objects[name]
	if !ok {
		return 0, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	return int64(len(data)), nil
}

func (m *inMemClient) Put(
	_ context.Context, name string, _ int64, r io.Reader,
) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.objects[name] = data
	return nil
}

// chunkName returns "chunk-<i>" for testing.
func chunkName(i int) string {
	return fmt.Sprintf("chunk-%d", i)
}

// ---- ChunkedSource tests ----

// TestChunkedSource_offsets verifies reads at various offsets produce correct
// content for a multi-chunk object.
func TestChunkedSource_offsets(t *testing.T) {
	// 25 bytes split into chunks of 10.
	data := []byte("abcdefghijklmnopqrstuvwxy")
	chunkSize := int64(10)

	m := newInMemClient()
	// Pre-populate chunks.
	m.objects["chunk-0"] = data[0:10]
	m.objects["chunk-1"] = data[10:20]
	m.objects["chunk-2"] = data[20:25]

	src := fileserver.NewChunkedSource(
		m, chunkName, int64(len(data)), chunkSize,
	)

	for _, tc := range []struct {
		name   string
		offset int64
		want   []byte
	}{
		{"offset_0", 0, data},
		{"mid_first_chunk", 5, data[5:]},
		{"chunk_boundary", 10, data[10:]},
		{"mid_second_chunk", 15, data[15:]},
		{"last_byte", int64(len(data)) - 1, data[len(data)-1:]},
		{"at_end", int64(len(data)), []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc, err := src.Open(context.Background(), tc.offset)
			testhelper.AssertErrorsIs(t, err, nil)
			defer rc.Close()
			got, err := io.ReadAll(rc)
			testhelper.AssertErrorsIs(t, err, nil)
			testhelper.AssertTrue(
				t, bytes.Equal(tc.want, got),
				"offset %d: want %q, got %q", tc.offset, tc.want, got,
			)
		})
	}
}

// TestChunkedSource_out_of_bounds verifies that Open with a negative or
// too-large offset returns an error.
func TestChunkedSource_out_of_bounds(t *testing.T) {
	src := fileserver.NewChunkedSource(newInMemClient(), chunkName, 100, 20)

	_, err := src.Open(context.Background(), -1)
	testhelper.AssertNonNilInterface(t, err)

	_, err = src.Open(context.Background(), 101)
	testhelper.AssertNonNilInterface(t, err)
}

// TestChunkedSource_one_get_per_chunk asserts that a sequential full read
// of an N-chunk object issues exactly N GET requests (one per chunk).
func TestChunkedSource_one_get_per_chunk(t *testing.T) {
	data := bytes.Repeat([]byte{0xAB}, 30)
	m := newInMemClient()
	m.objects["chunk-0"] = data[0:10]
	m.objects["chunk-1"] = data[10:20]
	m.objects["chunk-2"] = data[20:30]

	m.getLog = nil
	src := fileserver.NewChunkedSource(m, chunkName, 30, 10)

	rc, err := src.Open(context.Background(), 0)
	testhelper.AssertErrorsIs(t, err, nil)
	got, err := io.ReadAll(rc)
	rc.Close()
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertTrue(t, bytes.Equal(data, got), "content mismatch")

	// Each chunk should have been fetched exactly once.
	testhelper.AssertEq(t, 3, len(m.getLog))
}

// TestChunkedSource_zero_size verifies that a zero-size object can be opened
// at offset 0 and returns empty content.
func TestChunkedSource_zero_size(t *testing.T) {
	m := newInMemClient()
	src := fileserver.NewChunkedSource(m, chunkName, 0, 100)

	rc, err := src.Open(context.Background(), 0)
	testhelper.AssertErrorsIs(t, err, nil)
	got, _ := io.ReadAll(rc)
	rc.Close()
	testhelper.AssertEq(t, 0, len(got))
}

// ---- ChunkedSink tests ----

// TestChunkedSink_fresh_push uploads a 3-chunk object and verifies all chunks
// are present with the correct content.
func TestChunkedSink_fresh_push(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxy") // 25 bytes
	chunkSize := int64(10)

	m := newInMemClient()
	sink := fileserver.NewChunkedSink(m, chunkName, int64(len(data)), chunkSize)

	err := sink.Append(context.Background(), 0, bytes.NewReader(data))
	testhelper.AssertErrorsIs(t, err, nil)

	testhelper.AssertTrue(
		t, bytes.Equal(data[0:10], m.objects["chunk-0"]), "chunk-0 content",
	)
	testhelper.AssertTrue(
		t, bytes.Equal(data[10:20], m.objects["chunk-1"]), "chunk-1 content",
	)
	testhelper.AssertTrue(
		t, bytes.Equal(data[20:25], m.objects["chunk-2"]), "chunk-2 content",
	)
}

// TestChunkedSink_state_partial verifies State reports the byte offset of the
// complete prefix when k chunks are present.
func TestChunkedSink_state_partial(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxy") // 25 bytes
	chunkSize := int64(10)

	for _, k := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("%d_chunks_present", k), func(t *testing.T) {
			m := newInMemClient()
			// Put exactly k complete chunks.
			for i := range k {
				lo := i * 10
				hi := min(lo+10, len(data))
				m.objects[fmt.Sprintf("chunk-%d", i)] = data[lo:hi]
			}

			sink := fileserver.NewChunkedSink(
				m, chunkName, int64(len(data)), chunkSize,
			)
			offset, complete, err := sink.State(context.Background())
			testhelper.AssertErrorsIs(t, err, nil)
			testhelper.AssertEq(t, int64(k)*chunkSize, offset)
			testhelper.AssertFalse(
				t, complete,
				"should not be complete with %d/%d chunks", k, 3,
			)
		})
	}
}

// TestChunkedSink_state_complete verifies State reports complete when all
// chunks are present with the expected sizes.
func TestChunkedSink_state_complete(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxy") // 25 bytes
	m := newInMemClient()
	m.objects["chunk-0"] = data[0:10]
	m.objects["chunk-1"] = data[10:20]
	m.objects["chunk-2"] = data[20:25]

	sink := fileserver.NewChunkedSink(m, chunkName, int64(len(data)), 10)
	offset, complete, err := sink.State(context.Background())
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(len(data)), offset)
	testhelper.AssertTrue(
		t, complete, "all chunks present: should be complete",
	)
}

// TestChunkedSink_state_wrong_size verifies that a chunk with the wrong size
// is treated as the end of the complete prefix.
func TestChunkedSink_state_wrong_size(t *testing.T) {
	// 25 bytes, chunk 1 is present but wrong size.
	m := newInMemClient()
	m.objects["chunk-0"] = bytes.Repeat([]byte{1}, 10)
	m.objects["chunk-1"] = bytes.Repeat([]byte{2}, 7) // want 10

	sink := fileserver.NewChunkedSink(m, chunkName, 25, 10)
	offset, complete, err := sink.State(context.Background())
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(10), offset) // only chunk-0 counts
	testhelper.AssertFalse(
		t, complete, "wrong-size chunk: should not be complete",
	)
}

// TestChunkedSink_append_misaligned verifies that Append with a non-chunk-
// aligned offset returns an error.
func TestChunkedSink_append_misaligned(t *testing.T) {
	m := newInMemClient()
	sink := fileserver.NewChunkedSink(m, chunkName, 100, 10)
	err := sink.Append(context.Background(), 5, bytes.NewReader([]byte("data")))
	testhelper.AssertNonNilInterface(t, err)
	testhelper.AssertErrorContains(t, err, "not chunk-aligned")
}

// TestChunkedSink_temp_cleanup verifies that temp files written by Append are
// removed after the method returns, both on success and on error.
func TestChunkedSink_temp_cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	data := bytes.Repeat([]byte{0xCC}, 25)
	m := newInMemClient()
	sink := fileserver.NewChunkedSink(m, chunkName, 25, 10)

	err := sink.Append(context.Background(), 0, bytes.NewReader(data))
	testhelper.AssertErrorsIs(t, err, nil)

	// TMPDIR should be empty after a successful Append.
	entries, err := os.ReadDir(tmpDir)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, 0, len(entries))
}

// TestChunkedSink_zero_size verifies that a zero-size object roundtrips: a
// single chunk-0 of size 0 is written, and State reports complete.
func TestChunkedSink_zero_size(t *testing.T) {
	m := newInMemClient()
	sink := fileserver.NewChunkedSink(m, chunkName, 0, 100)

	err := sink.Append(context.Background(), 0, bytes.NewReader(nil))
	testhelper.AssertErrorsIs(t, err, nil)

	_, ok := m.objects["chunk-0"]
	testhelper.AssertTrue(t, ok, "chunk-0 must exist for zero-size object")
	testhelper.AssertEq(t, 0, len(m.objects["chunk-0"]))

	offset, complete, err := sink.State(context.Background())
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(0), offset)
	testhelper.AssertTrue(
		t, complete, "zero-size: should be complete after upload",
	)
}

// TestChunkedSink_commit_noop verifies that Commit returns nil without side
// effects.
func TestChunkedSink_commit_noop(t *testing.T) {
	m := newInMemClient()
	sink := fileserver.NewChunkedSink(m, chunkName, 10, 10)
	testhelper.AssertErrorsIs(t, sink.Commit(context.Background()), nil)
}

// TestChunkedSource_Sink_roundtrip verifies that a full push+pull cycle
// preserves data exactly, for several chunk sizes including a final short
// chunk.
func TestChunkedSource_Sink_roundtrip(t *testing.T) {
	for _, tc := range []struct {
		name      string
		totalSize int
		chunkSize int64
	}{
		{"exact_one_chunk", 10, 10},
		{"two_full_chunks", 20, 10},
		{"final_short_chunk", 25, 10},
		{"single_byte", 1, 100},
		{"large_chunk", 1024, 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.totalSize)
			for i := range data {
				data[i] = byte(i & 0xFF)
			}

			m := newInMemClient()
			sink := fileserver.NewChunkedSink(
				m, chunkName, int64(tc.totalSize), tc.chunkSize,
			)
			err := sink.Append(
				context.Background(), 0, bytes.NewReader(data),
			)
			testhelper.AssertErrorsIs(t, err, nil)

			src := fileserver.NewChunkedSource(
				m, chunkName, int64(tc.totalSize), tc.chunkSize,
			)
			rc, err := src.Open(context.Background(), 0)
			testhelper.AssertErrorsIs(t, err, nil)
			got, err := io.ReadAll(rc)
			rc.Close()
			testhelper.AssertErrorsIs(t, err, nil)
			testhelper.AssertTrue(
				t, bytes.Equal(data, got),
				"roundtrip %s: content mismatch", tc.name,
			)
		})
	}
}
