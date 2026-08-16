package fileserver_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/fileserver"
	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

// fakeServer is a minimal in-memory object store served via httptest.
type fakeServer struct {
	objects map[string][]byte
	// reqLog records every request method+path seen.
	reqLog []string
	// headersSeen records every request header "Key: Value" seen.
	headersSeen []string
}

func newFakeServer() *fakeServer {
	return &fakeServer{objects: make(map[string][]byte)}
}

func (s *fakeServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.reqLog = append(s.reqLog, r.Method+" "+r.URL.Path)
		for k, vs := range r.Header {
			for _, v := range vs {
				s.headersSeen = append(s.headersSeen, k+": "+v)
			}
		}

		// Strip leading slash to get object name.
		name := strings.TrimPrefix(r.URL.Path, "/")

		switch r.Method {
		case http.MethodHead:
			data, ok := s.objects[name]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			data, ok := s.objects[name]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			rangeHdr := r.Header.Get("Range")
			if rangeHdr == "" {
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
			// Parse "bytes=<from>-"
			var from int64
			_, err := fmt.Sscanf(rangeHdr, "bytes=%d-", &from)
			if err != nil || from < 0 || int(from) > len(data) {
				http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			body := data[from:]
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body)

		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			s.objects[name] = body
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func newTestClient(t *testing.T, srv *fakeServer) (*fileserver.HTTPClient, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	hdr := http.Header{}
	hdr.Set("X-Test-Auth", "secret-token")
	c := &fileserver.HTTPClient{
		BaseURL: u,
		Header:  hdr,
		Client:  ts.Client(),
	}
	return c, ts
}

// headerPresent checks that s.headersSeen contains "Key: Value".
func headerPresent(t *testing.T, s *fakeServer, kv string) {
	t.Helper()
	for _, h := range s.headersSeen {
		if strings.EqualFold(
			strings.SplitN(h, ":", 2)[0],
			strings.SplitN(kv, ":", 2)[0],
		) && strings.Contains(h, strings.SplitN(kv, ": ", 2)[1]) {
			return
		}
	}
	t.Fatalf("header %q not found in %v", kv, s.headersSeen)
}

// TestHTTPClient_Get_offset0 verifies a 200 response for offset == 0.
func TestHTTPClient_Get_offset0(t *testing.T) {
	srv := newFakeServer()
	srv.objects["myobj"] = []byte("hello world")
	c, _ := newTestClient(t, srv)

	rc, size, err := c.Get(context.Background(), "myobj", 0)
	testhelper.AssertErrorsIs(t, err, nil)
	defer rc.Close()
	testhelper.AssertEq(t, int64(len("hello world")), size)
	got, _ := io.ReadAll(rc)
	testhelper.AssertTrue(
		t, bytes.Equal([]byte("hello world"), got),
		"Get offset0: content mismatch",
	)
}

// TestHTTPClient_Get_offset_positive verifies a 206 response for offset > 0.
func TestHTTPClient_Get_offset_positive(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	srv := newFakeServer()
	srv.objects["alpha"] = data
	c, _ := newTestClient(t, srv)

	rc, size, err := c.Get(context.Background(), "alpha", 10)
	testhelper.AssertErrorsIs(t, err, nil)
	defer rc.Close()
	testhelper.AssertEq(t, int64(len(data)), size)
	got, _ := io.ReadAll(rc)
	testhelper.AssertTrue(t, bytes.Equal(data[10:], got), "Get offset>0: content mismatch")
}

// TestHTTPClient_Get_ignored_range verifies that a 200 response to a ranged
// request returns an error.
func TestHTTPClient_Get_ignored_range(t *testing.T) {
	// Server that always returns 200 regardless of Range header.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	c := &fileserver.HTTPClient{BaseURL: u, Client: ts.Client()}

	_, _, err := c.Get(context.Background(), "obj", 1)
	testhelper.AssertNonNilInterface(t, err)
	testhelper.AssertErrorContains(t, err, "server ignored range")
}

// TestHTTPClient_Get_notfound verifies that 404 wraps fs.ErrNotExist.
func TestHTTPClient_Get_notfound(t *testing.T) {
	srv := newFakeServer()
	c, _ := newTestClient(t, srv)

	_, _, err := c.Get(context.Background(), "absent", 0)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

// TestHTTPClient_Get_410_gone verifies that 410 also wraps fs.ErrNotExist.
func TestHTTPClient_Get_410_gone(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	c := &fileserver.HTTPClient{BaseURL: u, Client: ts.Client()}

	_, _, err := c.Get(context.Background(), "obj", 0)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

// TestHTTPClient_auth_header verifies that the static Header is present on
// every method.
func TestHTTPClient_auth_header(t *testing.T) {
	srv := newFakeServer()
	srv.objects["x"] = []byte("data")
	c, _ := newTestClient(t, srv)

	srv.headersSeen = nil
	_, _, _ = c.Get(context.Background(), "x", 0)
	headerPresent(t, srv, "X-Test-Auth: secret-token")

	srv.headersSeen = nil
	_, _ = c.Stat(context.Background(), "x")
	headerPresent(t, srv, "X-Test-Auth: secret-token")

	srv.headersSeen = nil
	_ = c.Put(context.Background(), "x", 4, bytes.NewReader([]byte("data")))
	headerPresent(t, srv, "X-Test-Auth: secret-token")
}

// TestHTTPClient_Put_content_length verifies that Content-Length is sent.
func TestHTTPClient_Put_content_length(t *testing.T) {
	var seenCL int64 = -1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCL = r.ContentLength
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	c := &fileserver.HTTPClient{BaseURL: u, Client: ts.Client()}

	data := []byte("hello world")
	err := c.Put(context.Background(), "obj", int64(len(data)), bytes.NewReader(data))
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(len(data)), seenCL)
}

// TestHTTPClient_Stat verifies that Stat returns the correct size.
func TestHTTPClient_Stat(t *testing.T) {
	srv := newFakeServer()
	srv.objects["bigobj"] = bytes.Repeat([]byte{0xAB}, 4096)
	c, _ := newTestClient(t, srv)

	size, err := c.Stat(context.Background(), "bigobj")
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(4096), size)
}

// TestHTTPClient_Stat_notfound verifies that absent objects wrap fs.ErrNotExist.
func TestHTTPClient_Stat_notfound(t *testing.T) {
	srv := newFakeServer()
	c, _ := newTestClient(t, srv)

	_, err := c.Stat(context.Background(), "absent")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

// TestHTTPClient_URL_escaping verifies that names containing '/', spaces, and
// '%' are correctly percent-encoded in the request URL.
func TestHTTPClient_URL_escaping(t *testing.T) {
	// Record the raw request path as seen by the server.
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use RequestURI to get the raw (un-decoded) path.
		seen = append(seen, r.RequestURI)
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	c := &fileserver.HTTPClient{BaseURL: u, Client: ts.Client()}

	for _, tc := range []struct {
		name    string
		wantSub string // expected substring in raw URL
	}{
		{"a/b", "%2F"},         // slash in name must be escaped
		{"hello world", "%20"}, // space must be escaped
		{"x%y", "%25"},         // percent must be escaped
	} {
		seen = nil
		_, _ = c.Stat(context.Background(), tc.name)
		if len(seen) == 0 {
			t.Fatalf("name %q: no request made", tc.name)
		}
		if !strings.Contains(seen[0], tc.wantSub) {
			t.Fatalf(
				"name %q: want %q in request URI, got %q",
				tc.name, tc.wantSub, seen[0],
			)
		}
	}
}

// doerFunc is a function-adapter that satisfies [fileserver.Doer].
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// TestHTTPClient_Doer_interface verifies that a custom Doer is called by
// HTTPClient and can inject a canned response.
func TestHTTPClient_Doer_interface(t *testing.T) {
	var calls int
	stub := doerFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		// Return a canned 200 response with Content-Length so Stat succeeds.
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Length", "42")
		rec.WriteHeader(http.StatusOK)
		return rec.Result(), nil
	})

	u, _ := url.Parse("http://stub.invalid/bucket")
	c := &fileserver.HTTPClient{BaseURL: u, Client: stub}

	size, err := c.Stat(context.Background(), "obj")
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(42), size)
	testhelper.AssertEq(t, 1, calls)

	// A second call through Get must also route through the Doer.
	stub2 := doerFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Length", "5")
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.WriteString("hello")
		return rec.Result(), nil
	})
	c.Client = stub2
	rc, sz, err := c.Get(context.Background(), "obj", 0)
	testhelper.AssertErrorsIs(t, err, nil)
	testhelper.AssertEq(t, int64(5), sz)
	_ = rc.Close()
	testhelper.AssertEq(t, 2, calls)
}

// assertNoSecret fails the test if the error message contains any of the
// provided secret strings.
func assertNoSecret(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	for _, s := range secrets {
		if strings.Contains(msg, s) {
			t.Fatalf(
				"error message must NOT contain secret %q but got: %s", s, msg,
			)
		}
	}
}

// TestHTTPClient_redaction_Get verifies that secrets in BaseURL (userinfo and
// query-string token) do not appear in the error returned by Get when the
// server is unreachable.
func TestHTTPClient_redaction_Get(t *testing.T) {
	u, err := url.Parse(
		"https://user:secretpass@127.0.0.1:1/bucket?X-Token=SECRET",
	)
	if err != nil {
		t.Fatal(err)
	}
	c := &fileserver.HTTPClient{BaseURL: u}

	_, _, err = c.Get(context.Background(), "obj", 0)
	testhelper.AssertNonNilInterface(t, err)
	assertNoSecret(t, err, "secretpass", "SECRET")
}

// TestHTTPClient_redaction_Stat verifies secret redaction for Stat.
func TestHTTPClient_redaction_Stat(t *testing.T) {
	u, err := url.Parse(
		"https://user:secretpass@127.0.0.1:1/bucket?X-Token=SECRET",
	)
	if err != nil {
		t.Fatal(err)
	}
	c := &fileserver.HTTPClient{BaseURL: u}

	_, err = c.Stat(context.Background(), "obj")
	testhelper.AssertNonNilInterface(t, err)
	assertNoSecret(t, err, "secretpass", "SECRET")
}

// TestHTTPClient_redaction_Put verifies secret redaction for Put.
func TestHTTPClient_redaction_Put(t *testing.T) {
	u, err := url.Parse(
		"https://user:secretpass@127.0.0.1:1/bucket?X-Token=SECRET",
	)
	if err != nil {
		t.Fatal(err)
	}
	c := &fileserver.HTTPClient{BaseURL: u}

	err = c.Put(
		context.Background(), "obj", 5,
		strings.NewReader("hello"),
	)
	testhelper.AssertNonNilInterface(t, err)
	assertNoSecret(t, err, "secretpass", "SECRET")
}

// TestHTTPClient_redaction_404_still_ErrNotExist verifies that a 404 still
// produces an error wrapping fs.ErrNotExist even after our redaction path.
func TestHTTPClient_redaction_404_still_ErrNotExist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	// Build a BaseURL with fake credentials so redactURLError would be
	// exercised if it were triggered — but here the HTTP layer succeeds and
	// only the status code triggers the error path.
	raw := ts.URL + "/bucket?X-Token=SECRET"
	u, _ := url.Parse(raw)
	u.User = url.UserPassword("user", "secretpass")
	c := &fileserver.HTTPClient{BaseURL: u, Client: ts.Client()}

	_, _, err := c.Get(context.Background(), "obj", 0)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
	// Status-code path does not include the URL at all, so no secrets leak.
	assertNoSecret(t, err, "secretpass", "SECRET")

	_, err = c.Stat(context.Background(), "obj")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist from Stat, got %v", err)
	}
	assertNoSecret(t, err, "secretpass", "SECRET")
}
