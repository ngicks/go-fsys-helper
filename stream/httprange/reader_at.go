package httprange

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ReadAt implements [io.ReaderAt].
//
// Every call is a request of its own, so calls may run concurrently and none
// of them leaves anything behind for the others. A read costs one HTTP round
// trip; see the package documentation for how to keep a sequential scan from
// paying that per buffer.
//
// EOF follows [io.SectionReader]: an offset at or past the size of the object
// returns (0, [io.EOF]) without asking the server anything, which for a
// zero-length object is every read. Below that, an empty p returns (0, nil),
// also without a request. A read reaching the last byte of the object returns
// (n, io.EOF) along with the bytes it got.
//
// A response describing an object other than the one the reader was built
// against fails with an error matching [ErrObjectChanged], and one carrying
// the whole entity rather than the range asked for fails with an error
// matching [ErrRangeIgnored]. A body ending early fails with
// [io.ErrUnexpectedEOF]; the bytes read so far are returned with it. Any other
// status the server answers with comes back as a [*StatusCodeError]. No error
// text ever contains the query, fragment or userinfo of the URL, nor anything
// from the configured headers.
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("httprange.ReaderAt.ReadAt: negative offset %d", off)
	}
	if off >= r.size {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	if avail := r.size - off; int64(len(p)) > avail {
		p = p[:avail]
	}

	req, err := r.newRequest()
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1))
	ifRange := ifRangeValue(r.meta.Load())
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, redactURLError(err)
	}
	defer drainAndClose(resp.Body)

	if err := checkContentEncoding(resp); err != nil {
		return 0, fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), off, err,
		)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		if ifRange != "" {
			// A conditional range answered in full is HTTP saying the validator
			// no longer matches, not that ranges are unsupported.
			return 0, fmt.Errorf(
				"%w: reading %s at offset %d: If-Range did not match",
				ErrObjectChanged, redactRawURL(r.url), off,
			)
		}
		return 0, fmt.Errorf(
			"%w: reading %s at offset %d",
			ErrRangeIgnored, redactRawURL(r.url), off,
		)
	default:
		return 0, &StatusCodeError{Code: resp.StatusCode}
	}

	if err := r.checkPartial(resp, off); err != nil {
		return 0, err
	}

	n, err := io.ReadFull(resp.Body, p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return n, fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), off, redactURLError(err),
		)
	}
	if off+int64(n) == r.size {
		return n, io.EOF
	}
	return n, nil
}

// Size returns the total size of the remote object in bytes. It is settled
// when the reader is built and does not change afterwards.
func (r *ReaderAt) Size() int64 { return r.size }

// Close stops the reader: it cancels the context handed to [New], which aborts
// the requests still in flight and fails every read from then on. There is
// nothing else to release, so it always reports success.
func (r *ReaderAt) Close() error {
	// A context.CancelFunc may be called any number of times, which is all
	// Close needs in order to be idempotent.
	r.cancel()
	return nil
}

// checkPartial decides whether a 206 really is the stretch of bytes that was
// asked for, out of the object the reader was built against, before any of its
// body is believed.
func (r *ReaderAt) checkPartial(resp *http.Response, off int64) error {
	start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), off, err,
		)
	}
	if start != off || total != r.size {
		return fmt.Errorf(
			"%w: reading %s at offset %d: got bytes from %d of %d, want from %d of %d",
			ErrObjectChanged, redactRawURL(r.url), off, start, total, off, r.size,
		)
	}

	// With the probe skipped this response is the first word on what the object
	// is. Several goroutines can arrive here at once, so the first one to land
	// sets the description and every other one is checked against that.
	m := r.meta.Load()
	if m == nil {
		r.meta.CompareAndSwap(nil, metaFromResponse(resp))
		m = r.meta.Load()
	}
	if reason := m.mismatch(resp); reason != "" {
		return fmt.Errorf(
			"%w: reading %s at offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), off, reason,
		)
	}
	return nil
}

// mismatch says what about resp contradicts m, or "" when nothing does. Each
// property is compared only where both sides state it: a response leaving out
// its validators, or a Doer not recording the request it answered, says
// nothing either way and is not held against the response.
func (m *objectMeta) mismatch(resp *http.Response) string {
	if got := resp.Header.Get("ETag"); got != "" && m.etag != "" && got != m.etag {
		return fmt.Sprintf("ETag %s, want %s", got, m.etag)
	}
	got := resp.Header.Get("Last-Modified")
	if got != "" && m.lastModified != "" && got != m.lastModified {
		return fmt.Sprintf("Last-Modified %q, want %q", got, m.lastModified)
	}
	if got := originOf(resp); got != "" && m.origin != "" && got != m.origin {
		return fmt.Sprintf("origin %s, want %s", got, m.origin)
	}
	return ""
}

// ifRangeValue returns the validator to send in If-Range, or "" when there is
// none worth sending. A weak ETag is not one: a server is free to ignore a
// weak validator in an If-Range, so sending it would look like protection
// while buying none, and the reader would rather lean on comparing what comes
// back. Nothing is pinned yet either when m is nil.
func ifRangeValue(m *objectMeta) string {
	if m == nil {
		return ""
	}
	if m.etag != "" && !strings.HasPrefix(m.etag, "W/") {
		return m.etag
	}
	return m.lastModified
}
