package httprange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The credential-looking parts of a URL that must never reach error text.
const (
	secretPassword = "hunter2"
	secretToken    = "secret123"
)

// conformantServer answers through [http.ServeContent], which gets ranges,
// validators and If-Range right on its own. What it serves can be swapped
// mid-test to stand for an object changing under a reader.
type conformantServer struct {
	*httptest.Server

	mu       sync.Mutex
	content  []byte
	etag     string
	modTime  time.Time
	requests []http.Header
}

// startConformantServer serves content with the validators a reader pins
// itself to: ServeContent emits neither an ETag nor a Last-Modified of its
// own, so the ETag is set here and the modification time is non-zero.
func startConformantServer(t *testing.T, content []byte) *conformantServer {
	t.Helper()

	s := &conformantServer{
		content: content,
		etag:    `"v1"`,
		modTime: time.Date(2024, time.May, 6, 7, 8, 9, 0, time.UTC),
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *conformantServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	content, etag, modTime := s.content, s.etag, s.modTime
	s.requests = append(s.requests, r.Header.Clone())
	s.mu.Unlock()

	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	// A reader of its own per request: ServeContent seeks the one it is
	// handed, and requests run concurrently.
	http.ServeContent(w, r, "", modTime, bytes.NewReader(content))
}

// swap replaces the object from the next request on.
func (s *conformantServer) swap(content []byte, etag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.content, s.etag = content, etag
}

func (s *conformantServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *conformantServer) requestHeaders() []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]http.Header(nil), s.requests...)
}

// startHandlerServer runs h, which is how a response ServeContent would never
// produce gets tested: a server has to be written by hand to misbehave.
func startHandlerServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()

	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

// deadServerURL returns the URL of a server that has stopped listening, so a
// request to it is refused rather than answered.
func deadServerURL(t *testing.T) string {
	t.Helper()

	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s.Close()
	return s.URL
}

// doerFunc adapts a function to [Doer], which is how a test stands in for a
// client without a server behind it.
type doerFunc func(req *http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// requestLog keeps the headers of every request that reached the handler it
// wraps, for tests asking what the reader put on the wire.
type requestLog struct {
	mu      sync.Mutex
	headers []http.Header
}

func (l *requestLog) wrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.headers = append(l.headers, r.Header.Clone())
		l.mu.Unlock()
		h(w, r)
	}
}

func (l *requestLog) snapshot() []http.Header {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]http.Header(nil), l.headers...)
}

// handleSequence answers requests with hs in order, staying on the last one
// once the list runs out.
func handleSequence(hs ...http.HandlerFunc) http.HandlerFunc {
	var (
		mu sync.Mutex
		n  int
	)
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		h := hs[min(n, len(hs)-1)]
		n++
		mu.Unlock()
		h(w, r)
	}
}

// handlePartial answers whatever single byte range was asked for with a
// well-formed 206, tagged with etag unless it is empty. It pays no attention
// to If-Range, so what it serves is decided by the caller alone.
func handlePartial(content []byte, etag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end := rangeBounds(r.Header.Get("Range"))
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

// handleWhole answers with the entire entity whatever range was asked for.
func handleWhole(content []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		_, _ = w.Write(content)
	}
}

// handleEmptyChunked answers with an entity of zero bytes that never says how
// long it is. Flushing the header before writing anything is what arranges
// that: Go's server would otherwise size the body it saw and send a length of
// zero, and it is the answer without one — chunked, as a server streaming its
// reply sends — that the reader can tell from a whole entity only by reading.
func handleEmptyChunked(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
}

func rangeBounds(v string) (start, end int64) {
	if _, err := fmt.Sscanf(v, "bytes=%d-%d", &start, &end); err != nil {
		panic(fmt.Sprintf("test server got Range %q: %v", v, err))
	}
	return start, end
}

func testContent(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i%251) + 1
	}
	return b
}

// secretURL rewrites base into the shape of a presigned URL, carrying both a
// userinfo password and a query token.
func secretURL(t *testing.T, base string) string {
	t.Helper()

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("url.Parse(%q) returned error: %v", base, err)
	}
	u.User = url.UserPassword("user", secretPassword)
	u.Path = "/obj"
	u.RawQuery = "token=" + secretToken
	return u.String()
}

func assertNoSecret(t *testing.T, err error) {
	t.Helper()

	for _, secret := range []string{secretPassword, secretToken} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error %q leaked %q", err, secret)
		}
	}
}

// mustProbe runs the probe a test wants to have happened, which is what pins
// the object where the reader would otherwise leave it to the first read.
func mustProbe(t *testing.T, r *ReaderAt) {
	t.Helper()

	if err := r.Probe(t.Context()); err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
}

// sameObject reports whether two snapshots say the same thing about the object
// itself. Header is left out of the comparison, and out of the snapshots a
// test writes by hand with it: it is what one response carried rather than
// anything the object is, so the tests it is the subject of state it
// themselves. It stands in for the == a snapshot holding a map cannot have.
func sameObject(got, want Metadata) bool {
	return got.ETag == want.ETag &&
		got.LastModified == want.LastModified &&
		got.Size == want.Size
}

// TestNew_noRequest states the whole of what construction does about the
// network: nothing, whatever the caller handed in.
func TestNew_noRequest(t *testing.T) {
	content := testContent(1024)

	for _, tc := range []struct {
		name string
		cfg  *Config
	}{
		{name: "nothing_known", cfg: nil},
		{name: "size_known", cfg: &Config{PriorKnowledge: Metadata{Size: int64(len(content))}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, content)

			r, err := New(t.Context(), s.URL, tc.cfg)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer r.Close()

			if got := s.requestCount(); got != 0 {
				t.Fatalf("New made %d requests, want 0", got)
			}
		})
	}
}

func TestProbe(t *testing.T) {
	const lastModified = "Mon, 06 May 2024 07:08:09 GMT"
	content := testContent(1024)
	size := int64(len(content))

	t.Run("settles_everything", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		want := Metadata{ETag: s.etag, LastModified: lastModified, Size: size}
		if got, ok := r.Metadata(); !sameObject(got, want) || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
		if got := s.requestCount(); got != 1 {
			t.Fatalf("Probe made %d requests, want 1", got)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			cfg  *Config
		}{
			{name: "etag", cfg: &Config{PriorKnowledge: Metadata{ETag: `"v2"`}}},
			{
				name: "last_modified",
				cfg:  &Config{PriorKnowledge: Metadata{LastModified: "Tue, 07 May 2024 07:08:09 GMT"}},
			},
			{name: "size", cfg: &Config{PriorKnowledge: Metadata{Size: size + 1}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := startConformantServer(t, content)

				r, err := New(t.Context(), s.URL, tc.cfg)
				if err != nil {
					t.Fatalf("New returned error: %v", err)
				}
				defer r.Close()

				if err := r.Probe(t.Context()); !errors.Is(err, ErrObjectChanged) {
					t.Fatalf("Probe returned %v, want %v", err, ErrObjectChanged)
				}
			})
		}
	})

	t.Run("partial_metadata", func(t *testing.T) {
		s := startConformantServer(t, content)

		// Half a description, and the half that is there is the half the
		// response is held to; the rest is what the probe is for.
		r, err := New(
			t.Context(),
			s.URL,
			&Config{PriorKnowledge: Metadata{LastModified: lastModified}},
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)

		want := Metadata{ETag: s.etag, LastModified: lastModified, Size: size}
		if got, ok := r.Metadata(); !sameObject(got, want) || !ok {
			t.Fatalf("Metadata() = (%+v, %t), want (%+v, true)", got, ok, want)
		}
	})

	t.Run("fires_again_when_verified", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)
		mustProbe(t, r)

		if got := s.requestCount(); got != 2 {
			t.Fatalf("two probes made %d requests, want 2", got)
		}

		s.swap(content, `"v2"`)

		if err := r.Probe(t.Context()); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("Probe over a re-tagged object = %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("reader_closed", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}

		if err := r.Probe(t.Context()); !errors.Is(err, context.Canceled) {
			t.Fatalf("Probe after Close = %v, want %v", err, context.Canceled)
		}
		if got := s.requestCount(); got != 0 {
			t.Fatalf("Probe after Close made %d requests, want 0", got)
		}
	})

	t.Run("own_context_canceled", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := r.Probe(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Probe with a canceled context = %v, want %v", err, context.Canceled)
		}
		// The reader itself is untouched by the context that call carried.
		mustProbe(t, r)
	})
}

func TestNew_configuredSize(t *testing.T) {
	content := testContent(256)
	s := startConformantServer(t, content)

	r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{Size: int64(len(content))}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	if got := s.requestCount(); got != 0 {
		t.Fatalf("New made %d requests with a configured size, want 0", got)
	}
	if got, _ := r.Metadata(); got.Size != int64(len(content)) {
		t.Fatalf("Metadata().Size = %d, want %d", got.Size, len(content))
	}

	buf := make([]byte, 16)
	n, err := r.ReadAt(buf, 0)
	if err != nil || n != len(buf) {
		t.Fatalf("ReadAt = (%d, %v), want (%d, <nil>)", n, err, len(buf))
	}
	if !bytes.Equal(buf, content[:len(buf)]) {
		t.Fatalf("ReadAt returned %x, want %x", buf, content[:len(buf)])
	}

	changed := testContent(len(content))
	for i := range changed {
		changed[i] ^= 0xff
	}
	s.swap(changed, `"v2"`)

	if _, err := r.ReadAt(buf, 32); !errors.Is(err, ErrObjectChanged) {
		t.Fatalf("ReadAt after the object changed = %v, want %v", err, ErrObjectChanged)
	}
}

// TestNew_savedValidators covers the resuming caller: validators saved from an
// earlier download are handed back through the configuration, and bytes of
// anything else are refused before they can be spliced onto what is already
// saved.
func TestNew_savedValidators(t *testing.T) {
	const savedETag = `"v1"`
	content := testContent(256)
	size := int64(len(content))

	t.Run("if_range_answered_in_full", func(t *testing.T) {
		s := startConformantServer(t, content)
		s.swap(content, `"v2"`)

		r, err := New(
			t.Context(),
			s.URL,
			&Config{PriorKnowledge: Metadata{Size: size, ETag: savedETag}},
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		assertNoBytesOfAnotherObject(t, r)
	})

	t.Run("etag_in_partial_response", func(t *testing.T) {
		// A server paying no attention to If-Range answers the range anyway, so
		// the saved validator is what the response gets held to.
		s := startHandlerServer(t, handlePartial(content, `"v2"`))

		r, err := New(
			t.Context(),
			s.URL,
			&Config{PriorKnowledge: Metadata{Size: size, ETag: savedETag}},
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		assertNoBytesOfAnotherObject(t, r)
	})

	t.Run("explicit_probe", func(t *testing.T) {
		s := startConformantServer(t, content)
		s.swap(content, `"v2"`)

		// No size either, so nothing here is known well enough to read
		// against; the probe is what turns that into an answer.
		r, err := New(t.Context(), s.URL, &Config{PriorKnowledge: Metadata{ETag: savedETag}})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		if err := r.Probe(t.Context()); !errors.Is(err, ErrObjectChanged) {
			t.Fatalf("Probe over a re-tagged object = %v, want %v", err, ErrObjectChanged)
		}
	})

	t.Run("sent_as_if_range", func(t *testing.T) {
		const lastModified = "Mon, 06 May 2024 07:08:09 GMT"

		for _, tc := range []struct {
			name         string
			etag         string
			lastModified string
			wantIfRange  string
		}{
			{name: "etag", etag: savedETag, wantIfRange: savedETag},
			{
				name: "weak_etag_falls_back", etag: `W/` + savedETag,
				lastModified: lastModified, wantIfRange: lastModified,
			},
			{name: "weak_etag_alone", etag: `W/` + savedETag},
			{
				name: "last_modified", lastModified: lastModified,
				wantIfRange: lastModified,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var reqLog requestLog
				// The object answers with no validators of its own, leaving what
				// the caller saved as the only thing the reader can be holding.
				s := startHandlerServer(t, reqLog.wrap(handlePartial(content, "")))

				r, err := New(t.Context(), s.URL, &Config{
					PriorKnowledge: Metadata{
						Size:         size,
						ETag:         tc.etag,
						LastModified: tc.lastModified,
					},
				})
				if err != nil {
					t.Fatalf("New returned error: %v", err)
				}
				defer r.Close()

				if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
					t.Fatalf("ReadAt returned error: %v", err)
				}

				headers := reqLog.snapshot()
				if len(headers) != 1 {
					t.Fatalf("server saw %d requests, want the read alone", len(headers))
				}
				if tc.wantIfRange == "" {
					if got := headers[0].Values("If-Range"); len(got) != 0 {
						t.Fatalf("read sent If-Range %q, want none at all", got)
					}
					return
				}
				if got := headers[0].Get("If-Range"); got != tc.wantIfRange {
					t.Fatalf("If-Range = %q, want %q", got, tc.wantIfRange)
				}
			})
		}
	})
}

// assertNoBytesOfAnotherObject states what a saved validator buys: the read
// fails and leaves the buffer as it found it, so nothing of whatever the server
// is serving now can end up next to what the caller saved earlier.
func assertNoBytesOfAnotherObject(t *testing.T, r *ReaderAt) {
	t.Helper()

	buf := make([]byte, 16)
	n, err := r.ReadAt(buf, 0)
	if !errors.Is(err, ErrObjectChanged) {
		t.Fatalf("ReadAt = (%d, %v), want %v", n, err, ErrObjectChanged)
	}
	if n != 0 || !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatalf("ReadAt = (%d, %v) with buffer %x, want no bytes touched", n, err, buf)
	}
}

func TestNew_invalidURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rawURL  string
		wantMsg string
	}{
		{name: "empty", rawURL: "", wantMsg: "empty url"},
		{
			name:    "unparsable",
			rawURL:  "http://example.com/%zz",
			wantMsg: "invalid URL escape",
		},
		{
			name:    "wrong_scheme",
			rawURL:  "ftp://user:" + secretPassword + "@example.com/p?token=" + secretToken,
			wantMsg: "is not http or https",
		},
		{name: "no_host", rawURL: "http://", wantMsg: "no host"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A URL that can never become a ranged GET is refused before
			// anything reaches the wire, so this client must stay untouched.
			client := doerFunc(func(req *http.Request) (*http.Response, error) {
				t.Errorf("New issued a request to %q", req.URL.Redacted())
				return nil, errors.New("no request was expected")
			})

			r, err := New(t.Context(), tc.rawURL, &Config{Client: client})
			if err == nil {
				r.Close()
				t.Fatal("New returned no error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("New returned %v, want it to say %q", err, tc.wantMsg)
			}
			assertNoSecret(t, err)
		})
	}
}

func TestProbe_rangeIgnored(t *testing.T) {
	content := testContent(128)
	s := startHandlerServer(t, handleWhole(content))

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	if err := r.Probe(t.Context()); !errors.Is(err, ErrRangeIgnored) {
		t.Fatalf("Probe returned %v, want %v", err, ErrRangeIgnored)
	}
}

func TestProbe_emptyObject(t *testing.T) {
	t.Run("unsatisfied_range", func(t *testing.T) {
		s := startHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Range", "bytes */0")
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		})
		assertEmptyObject(t, s.URL)
	})

	t.Run("whole_empty_entity", func(t *testing.T) {
		s := startConformantServer(t, nil)
		assertWholeEmptyEntity(t, s.URL, 0)
		assertEmptyObject(t, s.URL)
	})

	// The same answer from a server that streams its replies: nothing in the
	// response says how long the entity is, so being empty is something only
	// reading it can tell.
	t.Run("chunked_empty_entity", func(t *testing.T) {
		s := startHandlerServer(t, handleEmptyChunked)
		assertWholeEmptyEntity(t, s.URL, -1)
		assertEmptyObject(t, s.URL)
	})
}

// assertWholeEmptyEntity states the premise of the case it guards: an empty
// object is what http.ServeContent answers a range request over with a plain
// 200 rather than a 416, so the reader must take that answer for a size of
// zero instead of a server dropping the range. wantLength is what the response
// says about its own length, -1 being the answer that says nothing at all.
func assertWholeEmptyEntity(t *testing.T, rawURL string, wantLength int64) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest returned error: %v", err)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf(
			"ranged GET answered %d with %d bytes, want %d with none",
			resp.StatusCode, len(body), http.StatusOK,
		)
	}
	if resp.ContentLength != wantLength {
		t.Fatalf(
			"the answer declared a length of %d, want %d",
			resp.ContentLength, wantLength,
		)
	}
}

func assertEmptyObject(t *testing.T, rawURL string) {
	t.Helper()

	r, err := New(t.Context(), rawURL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	mustProbe(t, r)

	// Zero and settled, which is an empty object; the validators alongside it
	// are whatever the server happened to send.
	if got, ok := r.Metadata(); got.Size != 0 || !ok {
		t.Fatalf("Metadata() = (%+v, %t), want size 0 and true", got, ok)
	}
	n, err := r.ReadAt(make([]byte, 8), 0)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
	}
}

func TestProbe_contentEncoding(t *testing.T) {
	s := startHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Range", "bytes 0-0/128")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	})

	r, err := New(t.Context(), s.URL, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer r.Close()

	err = r.Probe(t.Context())
	if err == nil {
		t.Fatal("Probe returned no error")
	}
	if errors.Is(err, ErrRangeIgnored) {
		t.Fatalf("Probe returned %v, want an error other than %v", err, ErrRangeIgnored)
	}
	if !strings.Contains(err.Error(), "Content-Encoding") {
		t.Fatalf("Probe returned %v, want it to name Content-Encoding", err)
	}
}

// TestProbe_badResponse covers the answers to a request for byte zero that are
// no answer at all, each in the class its equivalent takes on a read: bytes
// other than the one asked for describe another object, and a 416 that is not
// the refusal an empty object owes is a status the reader cannot work with.
func TestProbe_badResponse(t *testing.T) {
	content := testContent(128)

	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		want     error
		wantCode int
	}{
		{
			// Neither the object nor the range is in question here; the
			// response simply does not say what it carries.
			name: "unknown_total",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-0/*")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte{0})
			},
		},
		{
			name: "no_content_range",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte{0})
			},
		},
		{
			name: "wrong_start",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 1-1/128")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte{0})
			},
			want: ErrObjectChanged,
		},
		{
			// One byte was asked for and a hundred came back, which is a
			// server deciding for itself what a range means.
			name: "over_grant",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-99/128")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(content[:100])
			},
			want: ErrObjectChanged,
		},
		{
			// Byte zero is there in every object holding any bytes at all, so
			// a refusal reporting a hundred and twenty-eight of them refuses a
			// byte the response itself says is there.
			name: "unsatisfied_within_the_object",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes */128")
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			want: ErrObjectChanged,
		},
		{
			name: "satisfiable_416",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-0/128")
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			wantCode: http.StatusRequestedRangeNotSatisfiable,
		},
		{
			name: "bare_416",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			},
			wantCode: http.StatusRequestedRangeNotSatisfiable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startHandlerServer(t, tc.handler)

			r, err := New(t.Context(), s.URL, nil)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer r.Close()

			err = r.Probe(t.Context())
			switch {
			case tc.want != nil:
				if !errors.Is(err, tc.want) {
					t.Fatalf("Probe returned %v, want %v", err, tc.want)
				}
			case tc.wantCode != 0:
				codeErr, ok := errors.AsType[*StatusCodeError](err)
				if !ok || codeErr.Code != tc.wantCode {
					t.Fatalf(
						"Probe returned %v, want a *StatusCodeError of %d",
						err, tc.wantCode,
					)
				}
			default:
				if err == nil {
					got, _ := r.Metadata()
					t.Fatalf("Probe returned no error, size %d", got.Size)
				}
			}
		})
	}
}

func TestProbe_statusError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		notFound bool
	}{
		{name: "not_found", code: http.StatusNotFound, notFound: true},
		{name: "gone", code: http.StatusGone, notFound: true},
		{name: "forbidden", code: http.StatusForbidden},
		{name: "server_error", code: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(tc.code), tc.code)
			})

			r, err := New(t.Context(), s.URL, nil)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer r.Close()

			err = r.Probe(t.Context())
			codeErr, ok := errors.AsType[*StatusCodeError](err)
			if !ok {
				t.Fatalf("Probe returned %v, want a *StatusCodeError", err)
			}
			if codeErr.Code != tc.code {
				t.Fatalf("Code = %d, want %d", codeErr.Code, tc.code)
			}
			if got := codeErr.NotFound(); got != tc.notFound {
				t.Fatalf("NotFound() = %t, want %t", got, tc.notFound)
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf(
					"Probe returned %v matching fs.ErrNotExist; a remote object is not a file",
					err,
				)
			}
		})
	}
}

func TestSecretsStayOutOfErrorText(t *testing.T) {
	content := testContent(128)

	t.Run("probe_status", func(t *testing.T) {
		s := startHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no such object", http.StatusNotFound)
		})

		r, err := New(t.Context(), secretURL(t, s.URL), nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		err = r.Probe(t.Context())
		if err == nil {
			t.Fatal("Probe returned no error")
		}
		assertNoSecret(t, err)
	})

	t.Run("probe_unreachable", func(t *testing.T) {
		r, err := New(t.Context(), secretURL(t, deadServerURL(t)), nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		err = r.Probe(t.Context())
		if err == nil {
			t.Fatal("Probe returned no error")
		}
		assertNoSecret(t, err)
	})

	t.Run("read_range_ignored", func(t *testing.T) {
		s := startHandlerServer(t, handleWhole(content))

		r, err := New(t.Context(), secretURL(t, s.URL), nil)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		_, err = r.ReadAt(make([]byte, 16), 0)
		if err == nil {
			t.Fatal("ReadAt returned no error")
		}
		assertNoSecret(t, err)
		if !strings.Contains(err.Error(), "/obj") {
			t.Fatalf("ReadAt returned %v, want it to name the redacted url", err)
		}
	})

	t.Run("stream_range_ignored", func(t *testing.T) {
		s := startHandlerServer(t, handleWhole(content))

		r, err := NewRange(t.Context(), secretURL(t, s.URL), 0, math.MaxInt64, nil)
		if err != nil {
			t.Fatalf("NewRange returned error: %v", err)
		}
		defer r.Close()

		_, err = r.ReadAt(make([]byte, 16), 0)
		if err == nil {
			t.Fatal("ReadAt returned no error")
		}
		assertNoSecret(t, err)
		// The words the stream's own failures are put in, which no read of a
		// reader built by New can produce.
		if !strings.Contains(err.Error(), "streaming") {
			t.Fatalf("ReadAt returned %v, want the stream's own wording", err)
		}
		if !strings.Contains(err.Error(), "/obj") {
			t.Fatalf("ReadAt returned %v, want it to name the redacted url", err)
		}
	})

	t.Run("read_unreachable", func(t *testing.T) {
		r, err := New(
			t.Context(),
			secretURL(t, deadServerURL(t)),
			&Config{PriorKnowledge: Metadata{Size: int64(len(content))}},
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		_, err = r.ReadAt(make([]byte, 16), 0)
		if err == nil {
			t.Fatal("ReadAt returned no error")
		}
		assertNoSecret(t, err)
	})
}

func TestNew_staticHeaders(t *testing.T) {
	const authorization = "Bearer sekrit-token"

	t.Run("sent_on_every_request", func(t *testing.T) {
		content := testContent(256)
		s := startConformantServer(t, content)

		r, err := New(t.Context(), s.URL, &Config{
			Header: http.Header{
				"Authorization": {authorization},
				// The headers deciding what a response means belong to the
				// reader; one handed in through the configuration is dropped.
				"Range": {"bytes=5-5"},
			},
		})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		mustProbe(t, r)
		if _, err := r.ReadAt(make([]byte, 16), 0); err != nil {
			t.Fatalf("ReadAt returned error: %v", err)
		}

		headers := s.requestHeaders()
		if len(headers) != 2 {
			t.Fatalf("server saw %d requests, want 2", len(headers))
		}
		for i, h := range headers {
			if got := h.Get("Authorization"); got != authorization {
				t.Fatalf("request %d Authorization = %q, want %q", i, got, authorization)
			}
		}
		if got := headers[0].Get("Range"); got != "bytes=0-0" {
			t.Fatalf("probe Range = %q, want the reader's own %q", got, "bytes=0-0")
		}
		if got := headers[1].Get("Range"); got != "bytes=0-15" {
			t.Fatalf("read Range = %q, want the reader's own %q", got, "bytes=0-15")
		}
	})

	t.Run("stays_out_of_error_text", func(t *testing.T) {
		content := testContent(128)
		s := startHandlerServer(t, handleWhole(content))

		r, err := New(t.Context(), s.URL, &Config{
			Header: http.Header{"Authorization": {authorization}},
		})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		defer r.Close()

		_, err = r.ReadAt(make([]byte, 16), 0)
		if err == nil {
			t.Fatal("ReadAt returned no error")
		}
		if strings.Contains(err.Error(), "sekrit-token") {
			t.Fatalf("error %q leaked the configured Authorization header", err)
		}
	})
}
