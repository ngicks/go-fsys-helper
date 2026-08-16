// Package fileserver provides a generic file-server client interface and a
// plain net/http implementation, along with chunked read/write adapters for
// large objects.
//
// Implementations of [Client] must be safe for concurrent use. The chunked
// adapters ([ChunkedSource] and [ChunkedSink]) satisfy the structural shape of
// fsutil's ResumableSource/ResumableSink without importing that module (zero
// new go.mod requirements; a compile-time assertion lives in the consumer).
package fileserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client is a minimal interface to a remote file/object server.
// Implementations must be safe for concurrent use.
type Client interface {
	// Get returns a reader for the object's content starting at offset,
	// plus the object's total size. Errors wrap [fs.ErrNotExist] when the
	// object is absent. Range support is REQUIRED of all implementations:
	// a server that does not honour byte-range requests must not be used.
	Get(
		ctx context.Context, name string, offset int64,
	) (io.ReadCloser, int64, error)

	// Stat reports the object's total size. Errors wrap [fs.ErrNotExist]
	// when the object is absent. Implementations issue an HTTP HEAD or
	// equivalent. Probes are cheap because callers use content-addressed
	// names.
	Stat(ctx context.Context, name string) (int64, error)

	// Put stores the whole object under name, overwriting any previous
	// content. size must be the exact byte count of r (Content-Length).
	// Partial writes must not become visible under name before Put returns
	// successfully; the commit marker pattern (write data chunks first,
	// write metadata last) relies on this.
	Put(ctx context.Context, name string, size int64, r io.Reader) error
}

// Doer executes a single HTTP request. *http.Client satisfies Doer.
// Supply a custom Doer to control redirect policy, inject auth signing
// (e.g. S3 SigV4), or add retries. When nil, http.DefaultClient is used.
//
// Redirects follow the Doer's policy. The static Header on [HTTPClient] is
// applied to the initial request only; a custom Doer or *http.Client with
// CheckRedirect can be supplied to restrict or sign redirected requests.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPClient is a [Client] backed by net/http.
// BaseURL is joined with the path-escaped object name for every request.
// Static Header values (e.g. Authorization) are copied onto every request.
// Client defaults to [http.DefaultClient] when nil.
//
// HTTPClient is safe for concurrent use because it is stateless after
// construction: every method creates its own *http.Request and reads/closes
// the response body before returning.
//
// Redirects follow the policy of the supplied Doer (or http.DefaultClient
// when Doer is nil). The static Header is applied to the initial request
// only; supply a custom Doer with its own redirect hook to sign or restrict
// redirected requests.
type HTTPClient struct {
	// BaseURL is the common URL prefix. Each object name is appended as an
	// additional path segment (path-escaped). Must not be nil.
	BaseURL *url.URL
	// Header contains static headers applied to every request (e.g.
	// "Authorization: Bearer …"). Values are never logged by this package.
	Header http.Header
	// Client is the underlying HTTP doer. Defaults to
	// [http.DefaultClient] when nil. *http.Client satisfies [Doer].
	Client Doer
}

func (c *HTTPClient) client() Doer {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

// objectURL returns the full URL for the named object.
// Name segments are path-escaped individually so that slashes in a name
// component remain escaped (i.e. a name "a/b" becomes "…/a%2Fb", not
// "…/a/b").
func (c *HTTPClient) objectURL(name string) string {
	base := c.BaseURL.String()
	// Strip trailing slash from base, then append /<escaped-name>.
	base = strings.TrimRight(base, "/")
	// Escape each segment of the name independently: a name that itself
	// contains '/' is a single logical name, not a path hierarchy.
	escaped := url.PathEscape(name)
	return base + "/" + escaped
}

// applyHeaders copies the static Header onto req.
func (c *HTTPClient) applyHeaders(req *http.Request) {
	for k, vs := range c.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
}

// redactURLError sanitizes a *url.Error so that credentials and query
// parameters embedded in the URL (e.g. presigned tokens in the query string
// or userinfo) are not exposed in log output or error messages.
//
// Guarantee: if err wraps a *url.Error, the returned error's message contains
// only the scheme, host, and path of the original URL — userinfo, query, and
// fragment are stripped. The underlying Err field is preserved unchanged so
// that errors.Is/As chains (e.g. context.Canceled, net errors, fs.ErrNotExist)
// continue to work normally. If err is not (or does not wrap) a *url.Error it
// is returned as-is.
func redactURLError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	// Parse and redact the URL embedded in the *url.Error.
	redacted := ue.URL
	if u, parseErr := url.Parse(ue.URL); parseErr == nil {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		redacted = u.String()
	}
	return &url.Error{
		Op:  ue.Op,
		URL: redacted,
		Err: ue.Err,
	}
}

// Get implements [Client].
//
// When offset == 0 the request has no Range header; a 200 or 206 response is
// accepted. When offset > 0 a "Range: bytes=<offset>-" header is sent and a
// 206 response is required — if the server returns 200 (meaning it ignored the
// range) Get returns an error of the form "server ignored range request" so
// that callers can detect servers that do not support byte ranges.
//
// 404 and 410 responses are mapped to errors that wrap [fs.ErrNotExist].
func (c *HTTPClient) Get(
	ctx context.Context, name string, offset int64,
) (io.ReadCloser, int64, error) {
	rawURL := c.objectURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"fileserver Get %q: build request: %w", name, redactURLError(err),
		)
	}
	c.applyHeaders(req)

	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"fileserver Get %q: %w", name, redactURLError(err),
		)
	}

	switch resp.StatusCode {
	case http.StatusOK: // 200
		if offset > 0 {
			// Server returned 200 to a ranged request — it ignored the Range
			// header. Drain and close the body to avoid leaking the connection.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil, 0, fmt.Errorf(
				"fileserver Get %q at offset %d: server ignored range request"+
					" (got 200, need 206); range support is required",
				name, offset,
			)
		}
		// offset == 0: full content — total size is Content-Length.
		total := resp.ContentLength
		return resp.Body, total, nil

	case http.StatusPartialContent: // 206
		// Total size is in Content-Range: bytes <from>-<to>/<total>.
		total, err := parseTotalFromContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil, 0, fmt.Errorf(
				"fileserver Get %q: parse Content-Range: %w", name, err,
			)
		}
		return resp.Body, total, nil

	case http.StatusNotFound, http.StatusGone: // 404, 410
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf(
			"fileserver Get %q: %w", name, fs.ErrNotExist,
		)

	default:
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf(
			"fileserver Get %q: unexpected status %d", name, resp.StatusCode,
		)
	}
}

// Stat implements [Client] via an HTTP HEAD request.
// Content-Length in the response is used as the object size.
// 404 and 410 wrap [fs.ErrNotExist].
func (c *HTTPClient) Stat(ctx context.Context, name string) (int64, error) {
	rawURL := c.objectURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf(
			"fileserver Stat %q: build request: %w", name, redactURLError(err),
		)
	}
	c.applyHeaders(req)

	resp, err := c.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("fileserver Stat %q: %w", name, redactURLError(err))
	}
	// HEAD responses have no body, but we call Close for correctness.
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		if resp.ContentLength < 0 {
			return 0, fmt.Errorf(
				"fileserver Stat %q: Content-Length missing in HEAD response",
				name,
			)
		}
		return resp.ContentLength, nil

	case http.StatusNotFound, http.StatusGone:
		return 0, fmt.Errorf("fileserver Stat %q: %w", name, fs.ErrNotExist)

	default:
		return 0, fmt.Errorf(
			"fileserver Stat %q: unexpected status %d", name, resp.StatusCode,
		)
	}
}

// Put implements [Client] via an HTTP PUT request with Content-Length.
// The entire body r is sent in a single request; size must equal the exact
// byte count.
func (c *HTTPClient) Put(
	ctx context.Context, name string, size int64, r io.Reader,
) error {
	rawURL := c.objectURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, r)
	if err != nil {
		return fmt.Errorf(
			"fileserver Put %q: build request: %w", name, redactURLError(err),
		)
	}
	req.ContentLength = size
	c.applyHeaders(req)

	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("fileserver Put %q: %w", name, redactURLError(err))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"fileserver Put %q: unexpected status %d", name, resp.StatusCode,
		)
	}
	return nil
}

// parseTotalFromContentRange parses the total size from a Content-Range
// header value of the form "bytes <from>-<to>/<total>".
func parseTotalFromContentRange(s string) (int64, error) {
	// Expected: "bytes 1024-2047/4096"
	if s == "" {
		return 0, errors.New("Content-Range header missing")
	}
	// Strip "bytes " prefix.
	s = strings.TrimPrefix(s, "bytes ")
	// Find the slash separating range from total.
	slashIdx := strings.LastIndex(s, "/")
	if slashIdx < 0 {
		return 0, fmt.Errorf("malformed Content-Range %q: no slash", s)
	}
	totalStr := s[slashIdx+1:]
	if totalStr == "*" {
		return 0, fmt.Errorf("Content-Range total is unknown (*)")
	}
	total, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Content-Range total parse: %w", err)
	}
	return total, nil
}
