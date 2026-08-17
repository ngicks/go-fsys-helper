package httprange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// newStreamReader returns a reader over rawURL carrying a lane for the view
// [base, base+length), which is what a caller declaring a range up front gets.
// The tests below drive that lane directly, at offsets into the object rather
// than into the view.
func newStreamReader(t *testing.T, rawURL string, cfg *Config, base, length int64) *ReaderAt {
	t.Helper()

	r, err := NewRange(context.Background(), rawURL, base, length, cfg)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	// A lane left open holds a response body, and a test server does not
	// finish closing while one is still outstanding.
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r
}

// openLane opens the lane the way the first read arriving in order does, under
// its lock.
func openLane(t *testing.T, r *ReaderAt) error {
	t.Helper()

	r.stream.mu.Lock()
	defer r.stream.mu.Unlock()
	return r.openStream()
}

func laneState(r *ReaderAt) streamState {
	r.stream.mu.Lock()
	defer r.stream.mu.Unlock()
	return r.stream.state
}

func assertLaneDead(t *testing.T, r *ReaderAt) {
	t.Helper()

	if laneState(r) != streamDead {
		t.Fatal("the lane is still alive, want it dead for good")
	}
}

// assertLaneServesNothing states the fall-back contract: the lane says it did
// not answer, and the caller makes a bounded request of its own.
func assertLaneServesNothing(t *testing.T, r *ReaderAt, off int64) {
	t.Helper()

	n, served, err := r.streamReadAt(make([]byte, 8), off)
	if n != 0 || served {
		t.Fatalf(
			"streamReadAt at offset %d = (%d, %t, %v), want it to serve nothing",
			off, n, served, err,
		)
	}
}

// handleStreamPartial answers a streaming range — the open-ended "bytes=N-" as
// well as the bounded form — with a well-formed 206, tagged with etag unless
// it is empty. Like handlePartial it pays no attention to If-Range, which is
// how a server handing back another object's bytes is arranged.
func handleStreamPartial(content []byte, etag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end := streamRangeBounds(r.Header.Get("Range"), int64(len(content)))
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		w.Header().Set(
			"Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)),
		)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}
}

// streamRangeBounds reads the inclusive bounds out of a Range header, taking
// an open end, and an end past the object, as its last byte.
func streamRangeBounds(v string, size int64) (start, end int64) {
	spec, ok := strings.CutPrefix(v, "bytes=")
	if !ok {
		panic(fmt.Sprintf("test server got Range %q", v))
	}
	first, last, ok := strings.Cut(spec, "-")
	if !ok {
		panic(fmt.Sprintf("test server got Range %q", v))
	}

	var err error
	if start, err = strconv.ParseInt(first, 10, 64); err != nil {
		panic(fmt.Sprintf("test server got Range %q: %v", v, err))
	}
	end = size - 1
	if last != "" {
		if end, err = strconv.ParseInt(last, 10, 64); err != nil {
			panic(fmt.Sprintf("test server got Range %q: %v", v, err))
		}
		end = min(end, size-1)
	}
	return start, end
}

// bodyStep is what one [scriptedBody.Read] hands back once its bytes run out.
type bodyStep struct {
	data []byte
	err  error
}

// scriptedBody hands out what a test wrote for it, step by step. How a body
// ends — an error partway, its last bytes reported along with io.EOF, or on
// the read after them — is the point of the tests using it, and a real server
// decides that for itself.
type scriptedBody struct {
	steps  []bodyStep
	closes atomic.Int64
}

func (b *scriptedBody) Read(p []byte) (int, error) {
	if len(b.steps) == 0 {
		return 0, io.EOF
	}
	step := b.steps[0]
	n := copy(p, step.data)
	step.data = step.data[n:]
	if len(step.data) > 0 {
		b.steps[0] = step
		return n, nil
	}
	b.steps = b.steps[1:]
	return n, step.err
}

func (b *scriptedBody) Close() error {
	b.closes.Add(1)
	return nil
}

// scriptedDoer answers every request with the one response handed to it, which
// is how a body no server would produce reaches the lane.
func scriptedDoer(status int, header http.Header, body io.ReadCloser) doerFunc {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     header,
			Body:       body,
			Request:    req,
		}, nil
	}
}

func TestStream_openRequest(t *testing.T) {
	content := testContent(128)

	for _, tc := range []struct {
		name        string
		cfg         *Config
		base        int64
		length      int64
		wantRange   string
		wantIfRange string
	}{
		{name: "whole_object", base: 0, length: math.MaxInt64, wantRange: "bytes=0-"},
		{name: "resumed", base: 64, length: math.MaxInt64, wantRange: "bytes=64-"},
		{name: "bounded", base: 16, length: 32, wantRange: "bytes=16-47"},
		{
			name: "bounded_past_known_end", cfg: &Config{Size: int64(len(content))},
			base: 96, length: 64, wantRange: "bytes=96-",
		},
		{
			name: "validator_pinned", cfg: &Config{ETag: `"v1"`},
			base: 0, length: math.MaxInt64,
			wantRange: "bytes=0-", wantIfRange: `"v1"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, content)
			r := newStreamReader(t, s.URL, tc.cfg, tc.base, tc.length)

			if err := openLane(t, r); err != nil {
				t.Fatalf("opening the stream returned error: %v", err)
			}

			headers := s.requestHeaders()
			if len(headers) != 1 {
				t.Fatalf("opening the stream made %d requests, want 1", len(headers))
			}
			if got := headers[0].Get("Range"); got != tc.wantRange {
				t.Fatalf("the stream asked for Range %q, want %q", got, tc.wantRange)
			}
			if got := headers[0].Get("If-Range"); got != tc.wantIfRange {
				t.Fatalf("the stream sent If-Range %q, want %q", got, tc.wantIfRange)
			}
		})
	}
}

// TestStream_open covers a 206 opening the lane: what the response grants is
// where the body ends, and its bytes come out in order from there.
func TestStream_open(t *testing.T) {
	content := testContent(128)
	size := int64(len(content))

	for _, tc := range []struct {
		name      string
		base      int64
		length    int64
		wantLimit int64
	}{
		{name: "whole_object", base: 0, length: math.MaxInt64, wantLimit: size},
		{name: "bounded", base: 16, length: 32, wantLimit: 48},
		// The view reaches past the object, so the server grants what exists
		// and the body ends at the object's own end.
		{name: "clamped_at_object_end", base: 96, length: 64, wantLimit: size},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, content)
			r := newStreamReader(t, s.URL, nil, tc.base, tc.length)

			if err := openLane(t, r); err != nil {
				t.Fatalf("opening the stream returned error: %v", err)
			}
			if got := r.stream.limit; got != tc.wantLimit {
				t.Fatalf("the stream was granted up to %d, want %d", got, tc.wantLimit)
			}

			buf := make([]byte, 8)
			n, served, err := r.streamReadAt(buf, tc.base)
			if !served || n != len(buf) || err != nil {
				t.Fatalf(
					"streamReadAt = (%d, %t, %v), want (%d, true, <nil>)",
					n, served, err, len(buf),
				)
			}
			want := content[tc.base : tc.base+int64(n)]
			if !bytes.Equal(buf[:n], want) {
				t.Fatalf("the stream served %x, want %x", buf[:n], want)
			}
			if got := r.stream.pos.Load(); got != tc.base+int64(n) {
				t.Fatalf("the stream stands at %d, want %d", got, tc.base+int64(n))
			}
		})
	}
}

// TestStream_open_pins states that the opening response is the reader's probe:
// a reader that knew nothing of the object holds its size and validators once
// the stream is open, at no request beyond the stream's own.
func TestStream_open_pins(t *testing.T) {
	content := testContent(128)
	s := startConformantServer(t, content)
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	if got, ok := r.Metadata(); ok {
		t.Fatalf("Metadata() = (%+v, %t) before the stream opened, want nothing settled", got, ok)
	}
	if err := openLane(t, r); err != nil {
		t.Fatalf("opening the stream returned error: %v", err)
	}

	got, ok := r.Metadata()
	if !ok || got.Size != int64(len(content)) || got.ETag != `"v1"` || got.LastModified == "" {
		t.Fatalf(
			"Metadata() = (%+v, %t), want the object the server described, settled",
			got, ok,
		)
	}
	if n := s.requestCount(); n != 1 {
		t.Fatalf("opening the stream cost %d requests, want 1", n)
	}
}

// TestStream_open_emptyView covers the answers leaving the lane with nothing
// to hand over: the view lies at or past the end of the object, or the object
// is empty. Both settle the size and read as a clean end.
func TestStream_open_emptyView(t *testing.T) {
	content := testContent(128)
	size := int64(len(content))

	for _, tc := range []struct {
		name     string
		content  []byte
		base     int64
		wantSize int64
	}{
		{name: "at_object_end", content: content, base: size, wantSize: size},
		{name: "past_object_end", content: content, base: size * 2, wantSize: size},
		{name: "empty_object", content: nil, base: 0, wantSize: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, tc.content)
			r := newStreamReader(t, s.URL, nil, tc.base, math.MaxInt64)

			if err := openLane(t, r); !errors.Is(err, io.EOF) {
				t.Fatalf("opening the stream returned %v, want %v", err, io.EOF)
			}
			assertLaneDead(t, r)
			if got, ok := r.Metadata(); got.Size != tc.wantSize || !ok {
				t.Fatalf(
					"Metadata() = (%+v, %t), want size %d, settled",
					got, ok, tc.wantSize,
				)
			}
			assertLaneServesNothing(t, r, tc.base)
		})
	}
}

// TestStream_open_rejects is the gate the opening response has to pass, which
// is the one every bounded read puts its own response through.
func TestStream_open_rejects(t *testing.T) {
	content := testContent(128)

	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		cfg      *Config
		want     error
		wantCode int
	}{
		{name: "range_ignored", handler: handleWhole(content), want: ErrRangeIgnored},
		{
			// A conditional range answered in full is HTTP saying the
			// validator no longer matches.
			name:    "if_range_did_not_match",
			handler: handleWhole(content),
			cfg:     &Config{ETag: `"v1"`},
			want:    ErrObjectChanged,
		},
		{
			name:    "another_validator",
			handler: handleStreamPartial(content, `"v2"`),
			cfg:     &Config{ETag: `"v1"`},
			want:    ErrObjectChanged,
		},
		{
			name:    "another_size",
			handler: handleStreamPartial(content, `"v1"`),
			cfg:     &Config{Size: 64},
			want:    ErrObjectChanged,
		},
		{
			// Granting less than the rest of the object would have the lane
			// read the end of the body as the end of the object.
			name: "short_grant",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-15/128")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[:16])
			},
			want: ErrObjectChanged,
		},
		{
			name: "wrong_start",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 8-127/128")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[8:])
			},
			want: ErrObjectChanged,
		},
		{
			name: "unsatisfied_within_the_object",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes */128")
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			want: ErrObjectChanged,
		},
		{
			name: "unsatisfied_another_size",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes */64")
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			cfg:  &Config{Size: 128},
			want: ErrObjectChanged,
		},
		{
			name:    "another_object_is_empty",
			handler: handleWhole(nil),
			cfg:     &Config{Size: 128},
			want:    ErrObjectChanged,
		},
		{
			// Neither the object nor the range is in question here; the
			// response simply does not say what it carries.
			name: "malformed_content_range",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-127/*")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content)
			},
		},
		{
			name: "bare_416",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			wantCode: http.StatusRequestedRangeNotSatisfiable,
		},
		{
			name: "status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			},
			wantCode: http.StatusForbidden,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startHandlerServer(t, tc.handler)
			r := newStreamReader(t, s.URL, tc.cfg, 0, math.MaxInt64)

			err := openLane(t, r)
			switch {
			case tc.want != nil:
				if !errors.Is(err, tc.want) {
					t.Fatalf("opening the stream returned %v, want %v", err, tc.want)
				}
			case tc.wantCode != 0:
				codeErr, ok := errors.AsType[*StatusCodeError](err)
				if !ok || codeErr.Code != tc.wantCode {
					t.Fatalf(
						"opening the stream returned %v, want a *StatusCodeError of %d",
						err, tc.wantCode,
					)
				}
			default:
				if err == nil {
					t.Fatal("opening the stream returned no error")
				}
			}
			assertLaneDead(t, r)
			assertLaneServesNothing(t, r, 0)
		})
	}
}

func TestStream_open_contentEncoding(t *testing.T) {
	content := testContent(128)
	s := startHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Range", "bytes 0-127/128")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content)
	})
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	err := openLane(t, r)
	if err == nil {
		t.Fatal("opening the stream returned no error")
	}
	if !strings.Contains(err.Error(), "Content-Encoding") {
		t.Fatalf("opening the stream returned %v, want it to name Content-Encoding", err)
	}
	assertLaneDead(t, r)
}

// TestStream_offPosition covers a read the lane is not holding — the shape a
// read losing the race for the position takes: it is served nothing, no
// request is spent on it, and the lane goes on holding its position for
// whoever reads there next.
func TestStream_offPosition(t *testing.T) {
	content := testContent(64)
	s := startConformantServer(t, content)
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	buf := make([]byte, 8)
	if n, served, err := r.streamReadAt(buf, 0); !served || n != len(buf) || err != nil {
		t.Fatalf(
			"streamReadAt = (%d, %t, %v), want (%d, true, <nil>)",
			n, served, err, len(buf),
		)
	}
	for _, off := range []int64{0, 16} {
		assertLaneServesNothing(t, r, off)
	}

	if laneState(r) != streamOpen {
		t.Fatal("the lane died over a read it merely did not serve")
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("the reads cost %d requests, want the stream's own", got)
	}
}

// TestStream_concurrentSamePosition covers several reads arriving at once at
// the position the lane holds: the lock lets one of them through, and the rest
// find the position taken and are served nothing, which is what sends them
// down the bounded path.
func TestStream_concurrentSamePosition(t *testing.T) {
	const (
		readers = 8
		bufLen  = 16
	)
	content := testContent(readers * bufLen)
	s := startConformantServer(t, content)
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	var (
		start  = make(chan struct{})
		wg     sync.WaitGroup
		served atomic.Int64
	)
	for range readers {
		wg.Go(func() {
			buf := make([]byte, bufLen)
			<-start
			n, ok, err := r.streamReadAt(buf, 0)
			if !ok {
				return
			}
			served.Add(1)
			if n != bufLen || err != nil {
				t.Errorf(
					"streamReadAt = (%d, %t, %v), want (%d, true, <nil>)",
					n, ok, err, bufLen,
				)
				return
			}
			if !bytes.Equal(buf, content[:bufLen]) {
				t.Errorf("the stream served %x, want %x", buf, content[:bufLen])
			}
		})
	}
	close(start)
	wg.Wait()

	if got := served.Load(); got != 1 {
		t.Fatalf("%d reads were served the same position, want exactly 1", got)
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("the reads cost %d requests, want the stream's own", got)
	}
}

// TestStream_consumeToEnd walks the view front to back the way the caller who
// declared it does: one request, in-order reads out of its body, and a clean
// end where the object ends.
func TestStream_consumeToEnd(t *testing.T) {
	content := testContent(96)
	s := startConformantServer(t, content)
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	var (
		got = make([]byte, 0, len(content))
		buf = make([]byte, 32)
	)
	for {
		off := int64(len(got))
		n, served, err := r.streamReadAt(buf, off)
		if !served {
			t.Fatalf("the lane stopped serving at offset %d", off)
		}
		got = append(got, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("streamReadAt at offset %d returned error: %v", off, err)
		}
	}

	if !bytes.Equal(got, content) {
		t.Fatalf("the stream served %d bytes, want the whole %d", len(got), len(content))
	}
	assertLaneDead(t, r)
	assertLaneServesNothing(t, r, int64(len(content)))
	if n := s.requestCount(); n != 1 {
		t.Fatalf("walking the view cost %d requests, want 1", n)
	}
}

// TestStream_cleanEnd covers both ways a body reaches the end of what it was
// granted — reporting it along with its last bytes, or on the read after them.
// Either is a whole object, and either leaves the read holding a buffer the
// stream had no more bytes to fill.
func TestStream_cleanEnd(t *testing.T) {
	content := testContent(16)

	for _, tc := range []struct {
		name  string
		steps []bodyStep
	}{
		{name: "with_the_last_bytes", steps: []bodyStep{{data: content, err: io.EOF}}},
		{name: "on_the_read_after", steps: []bodyStep{{data: content}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &scriptedBody{steps: tc.steps}
			r := newStreamReader(t, "http://example.test/obj", &Config{
				Client: scriptedDoer(
					http.StatusPartialContent,
					http.Header{"Content-Range": {"bytes 0-15/16"}},
					body,
				),
			}, 0, math.MaxInt64)

			buf := make([]byte, 2*len(content))
			n, served, err := r.streamReadAt(buf, 0)
			if !served || n != len(content) || !errors.Is(err, io.EOF) {
				t.Fatalf(
					"streamReadAt = (%d, %t, %v), want (%d, true, %v)",
					n, served, err, len(content), io.EOF,
				)
			}
			if !bytes.Equal(buf[:n], content) {
				t.Fatalf("the stream served %x, want %x", buf[:n], content)
			}
			assertLaneDead(t, r)
			if got := body.closes.Load(); got != 1 {
				t.Fatalf("the body was closed %d times, want once", got)
			}
		})
	}
}

// TestStream_lazyOpenFails states where a refused response surfaces when the
// lane opens on a read rather than ahead of one: the read carries it, since
// the bounded request it would otherwise fall back to would be refused just
// the same, one round trip later.
func TestStream_lazyOpenFails(t *testing.T) {
	s := startHandlerServer(t, handleWhole(testContent(128)))
	r := newStreamReader(t, s.URL, nil, 0, math.MaxInt64)

	n, served, err := r.streamReadAt(make([]byte, 8), 0)
	if n != 0 || !served || !errors.Is(err, ErrRangeIgnored) {
		t.Fatalf(
			"streamReadAt = (%d, %t, %v), want (0, true, %v)",
			n, served, err, ErrRangeIgnored,
		)
	}
	assertLaneDead(t, r)
}

// TestStream_openTransportError covers the request that never got an answer:
// the lane is as dead as one whose response was refused, since nothing here
// opens a second stream.
func TestStream_openTransportError(t *testing.T) {
	r := newStreamReader(t, deadServerURL(t), nil, 0, math.MaxInt64)

	if err := openLane(t, r); err == nil {
		t.Fatal("opening the stream against a dead server returned no error")
	}
	assertLaneDead(t, r)
	assertLaneServesNothing(t, r, 0)
}

// TestStream_bodyFails covers a stream dying halfway: the bytes it did get
// come back with the error, the lane is done for good, and the read after it
// is the caller's to make as a bounded request.
func TestStream_bodyFails(t *testing.T) {
	content := testContent(64)
	errReset := errors.New("connection reset")

	for _, tc := range []struct {
		name  string
		steps []bodyStep
		want  error
	}{
		{
			name:  "read_error",
			steps: []bodyStep{{data: content[:8]}, {err: errReset}},
			want:  errReset,
		},
		{
			// A body ending before the last byte it granted is the same
			// truncation a bounded read reports as an unexpected EOF.
			name:  "short_body",
			steps: []bodyStep{{data: content[:8], err: io.EOF}},
			want:  io.ErrUnexpectedEOF,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &scriptedBody{steps: tc.steps}
			r := newStreamReader(t, "http://example.test/obj", &Config{
				Client: scriptedDoer(
					http.StatusPartialContent,
					http.Header{"Content-Range": {
						fmt.Sprintf("bytes 0-%d/%d", len(content)-1, len(content)),
					}},
					body,
				),
			}, 0, math.MaxInt64)

			buf := make([]byte, 32)
			n, served, err := r.streamReadAt(buf, 0)
			if !served || n != 8 || !errors.Is(err, tc.want) {
				t.Fatalf(
					"streamReadAt = (%d, %t, %v), want (8, true, %v)",
					n, served, err, tc.want,
				)
			}
			if !bytes.Equal(buf[:n], content[:n]) {
				t.Fatalf("the stream served %x, want %x", buf[:n], content[:n])
			}
			assertLaneDead(t, r)
			if got := body.closes.Load(); got != 1 {
				t.Fatalf("the body was closed %d times, want once", got)
			}
			assertLaneServesNothing(t, r, int64(n))
		})
	}
}

// TestStream_kill states what killing a lane does, and that doing it again is
// nothing: the body is closed once and no read is served from then on.
func TestStream_kill(t *testing.T) {
	content := testContent(64)
	body := &scriptedBody{steps: []bodyStep{{data: content, err: io.EOF}}}
	r := newStreamReader(t, "http://example.test/obj", &Config{
		Client: scriptedDoer(
			http.StatusPartialContent,
			http.Header{"Content-Range": {"bytes 0-63/64"}},
			body,
		),
	}, 0, math.MaxInt64)

	if err := openLane(t, r); err != nil {
		t.Fatalf("opening the stream returned error: %v", err)
	}

	r.stream.kill()
	r.stream.kill()
	if got := body.closes.Load(); got != 1 {
		t.Fatalf("the body was closed %d times, want once", got)
	}
	assertLaneDead(t, r)
	assertLaneServesNothing(t, r, 0)
}
