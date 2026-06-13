package stream

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

func TestMultiReadAtSeekCloser_Close(t *testing.T) {
	buf := make([]byte, 10*1024+22)
	{
		r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024, 8994}, false))
		_, _ = io.ReadFull(r, buf)
		err := r.Close()
		testhelper.AssertErrorsIs(t, err, nil)
	}
	{
		sized := prepareSizedReader(randomBytes, []int{1024, 6789}, false)
		for i, r := range sized {
			r.R = &closable[*bytes.Reader]{R: r.R.(*bytes.Reader)}
			sized[i] = r
		}
		r := NewMultiReadAtSeekCloser(sized)
		_, _ = io.ReadFull(r, buf)
		err := r.Close()
		testhelper.AssertErrorsIs(t, err, nil)
		for i, r := range sized {
			testhelper.AssertTrue(
				t,
				r.R.(*closable[*bytes.Reader]).Closed.Load(),
				"Closed returned index %d",
				i,
			)
		}
	}
}

func TestMultiReadAtSeekCloser_read_all(t *testing.T) {
	for _, b := range []bool{false, true} {
		t.Run(useEofReaderAtTestCaseName(b), func(t *testing.T) {
			r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, b))
			var out bytes.Buffer
			buf := make([]byte, 1024)
			// prevent efficient methods like ReadFrom from being used.
			// Force it to be on boundary.
			_, err := io.CopyBuffer(onlyWrite{&out}, onlyRead{r}, buf)
			testhelper.AssertNilInterface(t, err)
			testhelper.AssertTrue(t,
				len(randomBytes) == out.Len(),
				"src len = %d, dst len = %d",
				len(randomBytes), out.Len(),
			)
			testhelper.AssertTrue(
				t,
				bytes.Equal(randomBytes, out.Bytes()),
				"bytes.Equal returned false",
			)
		})
	}
}

func TestMultiReadAtSeekCloser_ReadAt_reads_all(t *testing.T) {
	for _, b := range []bool{false, true} {
		t.Run(useEofReaderAtTestCaseName(b), func(t *testing.T) {
			r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, b))
			buf := make([]byte, len(randomBytes))
			n, err := r.ReadAt(buf, 0)
			testhelper.AssertTrue(
				t,
				err == nil || err == io.EOF,
				"err is not either of nil or io.EOF, but is %#v",
				err,
			)
			testhelper.AssertTrue(t,
				len(randomBytes) == n,
				"src len = %d, read = %d",
				len(randomBytes), n,
			)
			testhelper.AssertTrue(t, bytes.Equal(randomBytes, buf), "bytes.Equal returned false")
		})
	}
}

func TestMultiReadAtSeekCloser_ReadAt_reads_over_upper_limit(t *testing.T) {
	r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, false))
	buf := make([]byte, len(randomBytes))
	n, err := r.ReadAt(buf, 100)
	testhelper.AssertErrorsIs(t, err, io.EOF)
	testhelper.AssertTrue(t,
		len(randomBytes)-100 == n,
		"src len = %d, read = %d",
		len(randomBytes), n,
	)
	testhelper.AssertTrue(t, bytes.Equal(randomBytes[100:], buf[:n]), "bytes.Equal returned false")
}

func TestMultiReadAtSeekCloser_wrong_size(t *testing.T) {
	type testCase struct {
		name string // case name
		// difference between actual read size and alleged in
		// []SizedReaderAt. will be added to index 3.
		diff      int
		readAtLoc int64 // ReadAt offset where ReadAt return an error specified by err.
		err       error
	}
	for _, tc := range []testCase{
		{
			name:      "200bytes_more",
			diff:      200,
			readAtLoc: 1024*4 + 100,
			err:       io.ErrUnexpectedEOF,
		},
		{
			name:      "200bytes_less",
			diff:      -200,
			readAtLoc: 1024*3 + 700,
			err:       ErrInvalidSize,
		},
	} {
		t.Run("Read_"+tc.name, func(t *testing.T) {
			reader := prepareSizedReader(randomBytes, []int{1024}, false)

			sized := reader[3]
			sized.Size = sized.Size + int64(tc.diff)
			reader[3] = sized

			r := NewMultiReadAtSeekCloser(reader)
			var out bytes.Buffer
			buf := make([]byte, 1024)
			_, err := io.CopyBuffer(&out, r, buf)
			e := testhelper.AssertErrorsAs[*MultiReadError](t, err)
			testhelper.AssertErrorsIs(t, err, tc.err)
			t.Logf("err = %#v", e)
			t.Logf("internal = %#v", e.Err)
		})
		t.Run("ReadAt_"+tc.name, func(t *testing.T) {
			reader := prepareSizedReader(randomBytes, []int{1024}, false)

			sized := reader[3]
			sized.Size = sized.Size + int64(tc.diff)
			reader[3] = sized

			r := NewMultiReadAtSeekCloser(reader)
			buf := make([]byte, 1024)
			n, err := r.ReadAt(buf, tc.readAtLoc)
			t.Logf("ReadAt: %d", n)
			e := testhelper.AssertErrorsAs[*MultiReadError](t, err)
			testhelper.AssertErrorsIs(t, err, tc.err)
			t.Logf("err = %#v", e)
			t.Logf("internal = %#v", e.Err)
		})
	}
}

// TestMultiReadAtSeekCloser_wrapped_eof verifies that segments returning a
// wrapped io.EOF (fmt.Errorf("...: %w", io.EOF)) are treated as a clean
// end-of-segment: the concatenation must advance past each such segment and
// yield the full data without surfacing the wrapped error. Before the
// errors.Is fix the bare "== io.EOF" check aborted at the first segment.
func TestMultiReadAtSeekCloser_wrapped_eof(t *testing.T) {
	t.Run("Read", func(t *testing.T) {
		r := NewMultiReadAtSeekCloser(prepareWrappedEofReader(randomBytes, []int{1024}))
		var out bytes.Buffer
		buf := make([]byte, 1000) // unaligned to force boundary crossings
		// io.CopyBuffer with onlyRead/onlyWrite prevents ReadFrom/WriteTo
		// shortcuts so every byte flows through Read.
		_, err := io.CopyBuffer(onlyWrite{&out}, onlyRead{r}, buf)
		testhelper.AssertNilInterface(t, err)
		testhelper.AssertTrue(t,
			len(randomBytes) == out.Len(),
			"src len = %d, dst len = %d", len(randomBytes), out.Len(),
		)
		testhelper.AssertTrue(t, bytes.Equal(randomBytes, out.Bytes()), "bytes.Equal returned false")
	})
	t.Run("ReadAt", func(t *testing.T) {
		r := NewMultiReadAtSeekCloser(prepareWrappedEofReader(randomBytes, []int{1024}))
		buf := make([]byte, len(randomBytes))
		n, err := r.ReadAt(buf, 0)
		testhelper.AssertTrue(t,
			err == nil || err == io.EOF,
			"err is not either of nil or io.EOF, but is %#v", err,
		)
		testhelper.AssertEq(t, len(randomBytes), n)
		testhelper.AssertTrue(t, bytes.Equal(randomBytes, buf), "bytes.Equal returned false")
	})
}

// TestMultiReadAtSeekCloser_negative_size verifies that a negative
// SizedReaderAt.Size panics (D3: programmer error, no silent clamping).
func TestMultiReadAtSeekCloser_negative_size(t *testing.T) {
	defer func() {
		r := recover()
		testhelper.AssertNonNilInterface(t, r)
		msg, ok := r.(string)
		testhelper.AssertTrue(t, ok, "recover() is not a string: %#v", r)
		testhelper.AssertTrue(t,
			strings.Contains(msg, "negative Size"),
			"panic message %q does not mention negative Size", msg,
		)
	}()
	NewMultiReadAtSeekCloser([]SizedReaderAt{
		{R: bytes.NewReader([]byte("ok")), Size: 2},
		{R: bytes.NewReader([]byte("bad")), Size: -1},
	})
}
