package httprange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReaderAt_ReadAt(t *testing.T) {
	content := testContent(1024)
	size := int64(len(content))
	s := startConformantServer(t, content)

	r, err := New(context.Background(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

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

	r, err := New(context.Background(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	if got := r.Size(); got != int64(len(content)) {
		t.Fatalf("Size() = %d, want %d", got, len(content))
	}
	if _, err := r.ReadAt(make([]byte, 16), 0); !errors.Is(err, ErrRangeIgnored) {
		t.Fatalf("ReadAt returned %v, want %v", err, ErrRangeIgnored)
	}
}

func TestReaderAt_ReadAt_objectChanged(t *testing.T) {
	t.Run("if_range_answered_in_full", func(t *testing.T) {
		content := testContent(256)
		s := startConformantServer(t, content)

		r, err := New(context.Background(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

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

		r, err := New(context.Background(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

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

		r, err := New(context.Background(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

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

		r, err := New(context.Background(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		if _, err := r.ReadAt(make([]byte, 16), 64); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("ReadAt returned %v, want %v", err, ErrObjectChanged)
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

	r, err := New(context.Background(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

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

	r, err := New(context.Background(), s.URL, nil)
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

	r, err := New(context.Background(), s.URL, nil)
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

	r, err := New(context.Background(), s.URL, &Config{Size: int64(len(content))})
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
