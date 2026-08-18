package httprange

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

func TestReaderAt_ReadAt(t *testing.T) {
	content := testContent(1024)
	size := int64(len(content))
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	// The cases below count requests against a settled size, which is what the
	// probe settles; reads made before it are the unknown-size matrix's story.
	mustProbe(t, r)

	for _, tc := range []struct {
		name     string
		off      int64
		bufLen   int
		wantN    int
		wantErr  error
		wantReqs int
	}{
		{name: "start", off: 0, bufLen: 16, wantN: 16, wantReqs: 1},
		{name: "middle", off: 500, bufLen: 16, wantN: 16, wantReqs: 1},
		{name: "whole", off: 0, bufLen: 1024, wantN: 1024, wantErr: io.EOF, wantReqs: 1},
		{
			name: "tail_exact", off: size - 16, bufLen: 16,
			wantN: 16, wantErr: io.EOF, wantReqs: 1,
		},
		{
			name: "past_end_clamped", off: size - 8, bufLen: 16,
			wantN: 8, wantErr: io.EOF, wantReqs: 1,
		},
		{name: "at_size", off: size, bufLen: 16, wantErr: io.EOF},
		{name: "past_size", off: size * 2, bufLen: 16, wantErr: io.EOF},
		{name: "empty_buffer", off: 0, bufLen: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := s.requestCount()

			buf := make([]byte, tc.bufLen)
			n, err := r.ReadAt(buf, tc.off)
			if n != tc.wantN || !errors.Is(err, tc.wantErr) {
				t.Fatalf(
					"ReadAt(make([]byte, %d), %d) = (%d, %v), want (%d, %v)",
					tc.bufLen, tc.off, n, err, tc.wantN, tc.wantErr,
				)
			}
			if n > 0 && !bytes.Equal(buf[:n], content[tc.off:tc.off+int64(n)]) {
				t.Fatalf("ReadAt returned %x, want %x", buf[:n], content[tc.off:tc.off+int64(n)])
			}
			if got := s.requestCount() - before; got != tc.wantReqs {
				t.Fatalf("ReadAt made %d requests, want %d", got, tc.wantReqs)
			}
		})
	}

	t.Run("negative_offset", func(t *testing.T) {
		before := s.requestCount()

		n, err := r.ReadAt(make([]byte, 16), -1)
		if n != 0 || err == nil {
			t.Fatalf("ReadAt(make([]byte, 16), -1) = (%d, %v), want an error", n, err)
		}
		if errors.Is(err, io.EOF) {
			t.Fatalf("ReadAt at a negative offset returned %v, want it apart from EOF", err)
		}
		if got := s.requestCount() - before; got != 0 {
			t.Fatalf("ReadAt made %d requests, want 0", got)
		}
	})
}

func TestReaderAt_ReadAt_rangeIgnored(t *testing.T) {
	content := testContent(128)
	// The probe carries no validator, so no If-Range goes out and the whole
	// entity that comes back can only mean the server dropped the range.
	s := startHandlerServer(t, handleSequence(
		handlePartial(content, ""),
		handleWhole(content),
	))

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	if got, _ := r.Metadata(); got.Size != int64(len(content)) {
		t.Fatalf("Metadata().Size = %d, want %d", got.Size, len(content))
	}
	if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, ErrRangeIgnored) {
		t.Fatalf("ReadAt returned %v, want %v", err, ErrRangeIgnored)
	}
}

// TestReaderAt_ReadAt_verifiesLazily covers the reader that never probed: the
// first read's own response is what the validator the caller saved gets
// checked against, and no request is spent on checking it.
func TestReaderAt_ReadAt_verifiesLazily(t *testing.T) {
	content := testContent(256)
	var reqLog requestLog
	s := startHandlerServer(t, reqLog.wrap(handlePartial(content, `"v2"`)))

	r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{ETag: `"v1"`}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.ReadAt(buf, 0)
	if !errors.Is(err, ErrObjectChanged) {
		t.Fatalf("ReadAt = (%d, %v), want %v", n, err, ErrObjectChanged)
	}
	if n != 0 || !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatalf("ReadAt = (%d, %v) with buffer %x, want no bytes touched", n, err, buf)
	}
	if got := len(reqLog.snapshot()); got != 1 {
		t.Fatalf("the read cost %d requests, want the read alone", got)
	}
}

// TestReaderAt_ReadAt_unknownSize covers reads made before anything has said
// how large the object is: the request goes out as it was asked for, and the
// answer both settles the size and says where the object ends.
func TestReaderAt_ReadAt_unknownSize(t *testing.T) {
	content := testContent(256)
	size := int64(len(content))

	for _, tc := range []struct {
		name     string
		content  []byte
		off      int64
		wantN    int
		wantSize int64
	}{
		{name: "past_end", content: content, off: size + 64, wantSize: size},
		{name: "straddles_end", content: content, off: size - 8, wantN: 8, wantSize: size},
		{name: "empty_object", content: nil, off: 0, wantSize: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, tc.content)

			r, err := New(t.Context(), s.URL, nil)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer r.Close()

			buf := make([]byte, 16)
			n, err := r.ReadAt(buf, tc.off)
			if n != tc.wantN || !errors.Is(err, io.EOF) {
				t.Fatalf(
					"ReadAt(make([]byte, %d), %d) = (%d, %v), want (%d, %v)",
					len(buf), tc.off, n, err, tc.wantN, io.EOF,
				)
			}
			if n > 0 && !bytes.Equal(buf[:n], tc.content[tc.off:tc.off+int64(n)]) {
				t.Fatalf(
					"ReadAt returned %x, want %x",
					buf[:n], tc.content[tc.off:tc.off+int64(n)],
				)
			}
			if got, ok := r.Metadata(); got.Size != tc.wantSize || !ok {
				t.Fatalf(
					"Metadata() = (%+v, %t), want size %d and true",
					got, ok, tc.wantSize,
				)
			}
			if got := s.requestCount(); got != 1 {
				t.Fatalf("the read cost %d requests, want 1", got)
			}
		})
	}
}

// TestReaderAt_ReadAt_chunkedEmptyEntity covers the empty object answering a
// range with the whole entity and saying nothing about how long that entity
// is. Nothing probed beforehand, so this read is where the object turns out to
// be empty rather than the server turning out to ignore ranges, and reading the
// body is the only thing that can tell those apart.
func TestReaderAt_ReadAt_chunkedEmptyEntity(t *testing.T) {
	s := startHandlerServer(t, handleEmptyChunked)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	if n, err := r.ReadAt(make([]byte, 16), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
	}
	if got, ok := r.Metadata(); got.Size != 0 || !ok {
		t.Fatalf("Metadata() = (%+v, %t), want size 0 and true", got, ok)
	}
}

// TestReaderAt_wholeObjectWithoutSize walks the object front to back the way
// the package documentation says a scan may, with nothing having settled the
// size beforehand and the object's own end ending the walk.
func TestReaderAt_wholeObjectWithoutSize(t *testing.T) {
	content := testContent(1024)
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	got, err := io.ReadAll(
		bufio.NewReaderSize(io.NewSectionReader(r, 0, math.MaxInt64), 256),
	)
	if err != nil {
		t.Fatalf("reading the object returned error: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("read %d bytes differing from the %d served", len(got), len(content))
	}
}

func TestReaderAt_ReadAt_objectChanged(t *testing.T) {
	t.Run("if_range_answered_in_full", func(t *testing.T) {
		content := testContent(256)
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		changed := testContent(len(content))
		for i := range changed {
			changed[i] ^= 0xff
		}
		s.swap(changed, `"v2"`)

		_, err = r.ReadAt(make([]byte, 16), 0)
		if !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
		}
		// The whole entity came back because the validator no longer matched,
		// which is a changed object rather than a server dropping the range.
		if !strings.Contains(err.Error(), "If-Range") {
			t.Fatalf("ReadAt returned %v, want the If-Range mismatch", err)
		}
	})

	t.Run("etag_in_partial_response", func(t *testing.T) {
		content := testContent(256)
		s := startHandlerServer(t, handleSequence(
			handlePartial(content, `"v1"`),
			handlePartial(content, `"v2"`),
		))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("complete_length_drift", func(t *testing.T) {
		content := testContent(256)
		s := startHandlerServer(t, handleSequence(
			handlePartial(content, `"v1"`),
			handlePartial(testContent(320), `"v1"`),
		))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("wrong_offset", func(t *testing.T) {
		content := testContent(256)
		s := startHandlerServer(t, handleSequence(
			handlePartial(content, `"v1"`),
			func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("ETag", `"v1"`)
				w.Header().Set(
					"Content-Range",
					fmt.Sprintf("bytes 0-15/%d", len(content)),
				)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[:16])
			},
		))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		if _, err := r.ReadAt(make([]byte, 16), 64); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("short_range", func(t *testing.T) {
		content := testContent(256)
		s := startHandlerServer(t, handleSequence(
			handlePartial(content, `"v1"`),
			// A 206 announcing fewer bytes than were asked for. Taking it at
			// face value would leave the tail of the buffer holding whatever
			// the caller had put there.
			func(w http.ResponseWriter, r *http.Request) {
				start, _ := rangeBounds(r.Header.Get("Range"))
				w.Header().Set("ETag", `"v1"`)
				w.Header().Set(
					"Content-Range",
					fmt.Sprintf("bytes %d-%d/%d", start, start+7, len(content)),
				)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[start : start+8])
			},
		))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("over_grant", func(t *testing.T) {
		content := testContent(256)
		s := startHandlerServer(t, handleSequence(
			handlePartial(content, `"v1"`),
			// A 206 running to the end of the object however few bytes were
			// asked for, which is the over-grant that reads as a whole answer:
			// there is no room for those bytes in the buffer the caller
			// passed, and whatever room there is past it is not this package's
			// to write into.
			func(w http.ResponseWriter, r *http.Request) {
				start, _ := rangeBounds(r.Header.Get("Range"))
				w.Header().Set("ETag", `"v1"`)
				w.Header().Set(
					"Content-Range",
					fmt.Sprintf("bytes %d-%d/%d", start, len(content)-1, len(content)),
				)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[start:])
			},
		))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		buf := make([]byte, 16)
		n, err := r.ReadAt(buf, 0)
		if !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt = (%d, %v), want %v", n, err, ErrObjectChanged)
		}
		if n != 0 || !bytes.Equal(buf, make([]byte, len(buf))) {
			t.Fatalf("ReadAt = (%d, %v) with buffer %x, want no bytes touched", n, err, buf)
		}
	})
}

// TestReaderAt_ReadAt_originChanged pins a read to the origin the probe
// settled on, which is the far end of any redirect rather than the URL asked
// for.
func TestReaderAt_ReadAt_originChanged(t *testing.T) {
	content := testContent(256)

	// The redirect target answers exactly as the probe did, validators
	// included, so the origin is the only thing a read can find fault with.
	elsewhere := startHandlerServer(t, handlePartial(content, `"v1"`))
	s := startHandlerServer(t, handleSequence(
		handlePartial(content, `"v1"`),
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
		},
	))

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	_, err = r.ReadAt(make([]byte, 16), 0)
	if !errors.Is(err, ErrObjectChanged) {
		t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
	}
	if !strings.Contains(err.Error(), "origin") {
		t.Fatalf("ReadAt returned %v, want the origin mismatch", err)
	}
}

func TestReaderAt_ReadAt_statusError(t *testing.T) {
	content := testContent(256)
	// The object is there for the probe and gone by the time it is read.
	s := startHandlerServer(t, handleSequence(
		handlePartial(content, `"v1"`),
		func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no such object", http.StatusNotFound)
		},
	))

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	_, err = r.ReadAt(make([]byte, 16), 0)
	codeErr, ok := errors.AsType[*StatusCodeError](err)
	if !ok {
		t.Fatalf("ReadAt returned %v, want a *StatusCodeError", err)
	}
	if codeErr.Code != http.StatusNotFound {
		t.Fatalf("Code = %d, want %d", codeErr.Code, http.StatusNotFound)
	}
	if !codeErr.NotFound() {
		t.Fatalf("NotFound() = false, want true for status %d", codeErr.Code)
	}
}

func TestReaderAt_ReadAt_weakETag(t *testing.T) {
	const (
		weakETag     = `W/"v1"`
		lastModified = "Mon, 06 May 2024 07:08:09 GMT"
	)
	content := testContent(256)

	// A weak validator has to be served by hand: http.ServeContent sends a
	// Last-Modified whenever it has a modification time, and one case here
	// needs a response carrying nothing but the weak ETag.
	serveWeak := func(lastMod string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", weakETag)
			if lastMod != "" {
				w.Header().Set("Last-Modified", lastMod)
			}
			start, end := rangeBounds(r.Header.Get("Range"))
			w.Header().Set(
				"Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)),
			)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[start : end+1])
		}
	}

	for _, tc := range []struct {
		name         string
		lastModified string
		wantIfRange  string
	}{
		{
			name:         "falls_back_to_last_modified",
			lastModified: lastModified,
			wantIfRange:  lastModified,
		},
		{name: "nothing_left_to_send"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reqLog requestLog
			s := startHandlerServer(t, reqLog.wrap(serveWeak(tc.lastModified)))

			r, err := New(t.Context(), s.URL, nil)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer r.Close()

			mustProbe(t, r)
			if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
				t.Fatalf("ReadAt returned error: %v", err)
			}

			headers := reqLog.snapshot()
			if len(headers) != 2 {
				t.Fatalf("server saw %d requests, want 2", len(headers))
			}
			// A server may ignore a weak validator in an If-Range and answer
			// with the whole entity, so the reader must never send one.
			if got := headers[1].Get("If-Range"); strings.Contains(got, weakETag) {
				t.Fatalf("read If-Range = %q, want the weak ETag left out", got)
			}
			if tc.wantIfRange == "" {
				if got := headers[1].Values("If-Range"); len(got) != 0 {
					t.Fatalf("read sent If-Range %q, want none at all", got)
				}
				return
			}
			if got := headers[1].Get("If-Range"); got != tc.wantIfRange {
				t.Fatalf("read If-Range = %q, want %q", got, tc.wantIfRange)
			}
		})
	}
}

func TestReaderAt_Metadata(t *testing.T) {
	const (
		etag         = `"v1"`
		lastModified = "Mon, 06 May 2024 07:08:09 GMT"
	)
	content := testContent(256)
	size := int64(len(content))

	t.Run("config_pre_pin", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, &Config{
			PriorKnowledge: Metadata{
				Size:         size,
				ETag:         etag,
				LastModified: lastModified,
			},
		})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		want := Metadata{ETag: etag, LastModified: lastModified, Size: size}
		if got, ok := r.Metadata(); got != want || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
		if got := s.requestCount(); got != 0 {
			t.Fatalf("Metadata() cost %d requests, want 0", got)
		}
	})

	t.Run("size_alone", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{Size: size}})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		// A size the caller vouched for is known without the object it belongs
		// to being pinned, which is what ok reports on.
		want := Metadata{Size: size}
		if got, ok := r.Metadata(); got != want || ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, false)", got, ok, want)
		}
	})

	t.Run("nothing_known", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		// Zero and not ok, which is how an object nobody has looked at yet
		// tells itself apart from an empty one.
		want := Metadata{}
		if got, ok := r.Metadata(); got != want || ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, false)", got, ok, want)
		}
	})

	t.Run("from_probe", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		want := Metadata{
			ETag:         s.etag,
			LastModified: s.modTime.UTC().Format(http.TimeFormat),
			Size:         size,
		}
		if got, ok := r.Metadata(); got != want || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
	})

	t.Run("response_without_validators", func(t *testing.T) {
		// Nothing the response carries says which object this is, so what
		// settles the description is the response having happened at all.
		s := startHandlerServer(t, handlePartial(content, ""))

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
			t.Fatalf("ReadAt returned error: %v", err)
		}

		want := Metadata{Size: size}
		if got, ok := r.Metadata(); got != want || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
	})

	t.Run("completed_by_first_response", func(t *testing.T) {
		servePartial := func(etag string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Last-Modified", lastModified)
				handlePartial(content, etag)(w, r)
			}
		}
		s := startHandlerServer(t, handleSequence(
			servePartial(etag),
			servePartial(`"v2"`),
		))

		// Half a description: the caller saved a Last-Modified and no ETag.
		r, err := New(t.Context(), s.URL, &Config{
			PriorKnowledge: Metadata{
				Size:         size,
				LastModified: lastModified,
			},
		})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
			t.Fatalf("ReadAt returned error: %v", err)
		}

		want := Metadata{ETag: etag, LastModified: lastModified, Size: size}
		if got, ok := r.Metadata(); got != want || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
		if _, err := r.ReadAt(make([]byte, 16), 32); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt after the ETag changed = %v, want %v", err, ErrObjectChanged)
		}
	})
}

func TestReaderAt_ReadAt_shortBody(t *testing.T) {
	content := testContent(256)
	s := startHandlerServer(t, handleSequence(
		handlePartial(content, `"v1"`),
		func(w http.ResponseWriter, r *http.Request) {
			start, end := rangeBounds(r.Header.Get("Range"))
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set(
				"Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)),
			)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[start : start+2])
		},
	))

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	n, err := r.ReadAt(make([]byte, 16), 0)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadAt = (%d, %v), want %v", n, err, io.ErrUnexpectedEOF)
	}
	if n != 2 {
		t.Fatalf("ReadAt read %d bytes, want the 2 the server sent", n)
	}
}

func TestReaderAt_ReadAt_parallel(t *testing.T) {
	content := testContent(8192)
	size := int64(len(content))
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(seed))
			for range 8 {
				off := rng.Int63n(size)
				buf := make([]byte, 1+rng.Intn(512))
				n, err := r.ReadAt(buf, off)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Errorf("ReadAt(make([]byte, %d), %d) returned error: %v", len(buf), off, err)
					return
				}
				if !bytes.Equal(buf[:n], content[off:off+int64(n)]) {
					t.Errorf("ReadAt(make([]byte, %d), %d) read the wrong bytes", len(buf), off)
					return
				}
			}
		}(int64(i))
	}
	wg.Wait()
}

// TestReaderAt_ReadAt_concurrentFirstRead covers the reader built with a size
// it was handed: nothing describes the object until a read comes back, and
// every one of these goroutines is racing to be the read that settles it.
func TestReaderAt_ReadAt_concurrentFirstRead(t *testing.T) {
	const (
		readers = 8
		bufLen  = 64
	)
	content := testContent(readers * bufLen)
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{Size: int64(len(content))}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range readers {
		wg.Add(1)
		go func(off int64) {
			defer wg.Done()

			buf := make([]byte, bufLen)
			<-start
			n, err := r.ReadAt(buf, off)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Errorf("ReadAt(make([]byte, %d), %d) returned error: %v", bufLen, off, err)
				return
			}
			if !bytes.Equal(buf[:n], content[off:off+int64(n)]) {
				t.Errorf("ReadAt(make([]byte, %d), %d) read the wrong bytes", bufLen, off)
			}
		}(int64(i) * bufLen)
	}
	close(start)
	wg.Wait()

	changed := testContent(len(content))
	for i := range changed {
		changed[i] ^= 0xff
	}
	s.swap(changed, `"v2"`)

	if _, err := r.ReadAt(make([]byte, bufLen), 0); !errors.Is(err, ErrObjectChanged) {
		t.Fatalf(
			"ReadAt after the object changed = %v, want %v; the reads above left it unpinned",
			err, ErrObjectChanged,
		)
	}
}

// TestReaderAt_iotest walks the [io.Reader] contract over the reader, which
// asks for every offset in the object one small piece at a time.
func TestReaderAt_iotest(t *testing.T) {
	// Each of those pieces costs an HTTP round trip, so the object is kept
	// small enough for the walk to stay quick.
	content := testContent(1024)
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	meta, _ := r.Metadata()
	if err := iotest.TestReader(io.NewSectionReader(r, 0, meta.Size), content); err != nil {
		t.Fatalf("iotest.TestReader returned error: %v", err)
	}
}

func TestReaderAt_parentContextCanceled(t *testing.T) {
	content := testContent(256)
	s := startConformantServer(t, content)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := New(ctx, s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
		t.Fatalf("ReadAt returned error: %v", err)
	}

	cancel()

	if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"ReadAt after the parent context was canceled = %v, want %v",
			err,
			context.Canceled,
		)
	}
}

func TestReaderAt_Close(t *testing.T) {
	content := testContent(256)
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}

	if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadAt after Close = %v, want %v", err, context.Canceled)
	}
}

func TestReaderAt_CloseDuringRead(t *testing.T) {
	content := testContent(256)
	arrived := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	// The size is configured so that the read below is the only request the
	// reader makes, which is the one the handler holds on to.
	s := startHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})

	r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{Size: int64(len(content))}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := r.ReadAt(make([]byte, 16), 0)
		done <- err
	}()

	<-arrived
	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the in-flight ReadAt returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the in-flight ReadAt did not return after Close")
	}
}
