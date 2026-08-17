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
// text this package writes contains the query, fragment or userinfo of the
// URL, nor anything from the configured headers, and a transport error naming
// the URL is rewritten the same way as far as [Doer] leaves room for.
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

	if err := r.checkPartial(resp, off, int64(len(p))); err != nil {
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

// Metadata is a snapshot of what a [ReaderAt] has pinned about the remote
// object: the validators later responses are held to, and Size, the total size
// of the object in bytes.
//
// The origin the reader pins alongside these is deliberately left out. It
// guards against a redirect landing a later request on some other server, and
// says nothing about which object this is, so it is not something to save and
// hand back through [Config].
type Metadata struct {
	ETag         string
	LastModified string
	Size         int64
}

// Metadata reports what the reader has pinned about the remote object, and
// whether the identity of that object is settled: false until a response has
// been seen, when the configuration carried no validator of its own either.
// Size is filled in regardless, since a size the caller vouched for is known
// without any of this being pinned.
//
// Saving the snapshot is how a download resumes later: hand the validators
// back through [Config] and the reader refuses to splice bytes of another
// object onto the ones already saved. It may be called while reads are in
// flight, never waits for them, and issues no request of its own.
func (r *ReaderAt) Metadata() (Metadata, bool) {
	m := r.meta.Load()
	if m == nil {
		return Metadata{Size: r.size}, false
	}
	return Metadata{ETag: m.etag, LastModified: m.lastModified, Size: r.size}, true
}

// Close stops the reader: it cancels a context derived from the one handed to
// [New], which aborts the requests still in flight and fails every read from
// then on. The context the caller passed to New is left untouched. There is
// nothing else to release, so it always reports success.
func (r *ReaderAt) Close() error {
	// A context.CancelFunc may be called any number of times, which is all
	// Close needs in order to be idempotent.
	r.cancel()
	return nil
}

// checkPartial decides whether a 206 really is the stretch of bytes that was
// asked for, out of the object the reader was built against, before any of its
// body is believed. length is how many bytes the request asked for, after the
// clamp against the end of the object.
func (r *ReaderAt) checkPartial(resp *http.Response, off, length int64) error {
	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), off, err,
		)
	}
	wantEnd := off + length - 1
	if start != off || end != wantEnd || total != r.size {
		return fmt.Errorf(
			"%w: reading %s at offset %d: got bytes %d-%d of %d, want %d-%d of %d",
			ErrObjectChanged, redactRawURL(r.url), off,
			start, end, total, off, wantEnd, r.size,
		)
	}

	// Whatever nothing has pinned yet this response gets to say: with the probe
	// skipped it is the first word on what the object is, and it completes a
	// description the caller only knew part of.
	if reason := r.pinOrVerify(resp); reason != "" {
		return fmt.Errorf(
			"%w: reading %s at offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), off, reason,
		)
	}
	return nil
}

// mismatch says what about resp contradicts m, or "" when nothing does. Each
// property is compared only where both sides state it: a response leaving out
// its validators, a Doer not recording the request it answered, or a
// description holding nothing yet says nothing either way and is not held
// against the response.
func (m *objectMeta) mismatch(resp *http.Response) string {
	if m == nil {
		return ""
	}
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
