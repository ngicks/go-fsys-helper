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
// Every call issues a Range Request, so callers may use a large buffer and/or
// their own buffering mechanism to prevent too many requests.
//
// The first request also carries the check [ReaderAt.Probe] describes,
// over its own response.
//
// It returns every error that the transport has returned plus
// [ErrObjectChanged], [ErrRangeIgnored] and [*StatusCodeError].
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("httprange.ReaderAt.ReadAt: negative offset %d", off)
	}
	p, abs, inside := r.locate(p, off)
	if !inside {
		return 0, io.EOF
	}
	if size, known := r.knownSize(); known {
		if abs >= size {
			return 0, io.EOF
		}
		if avail := size - abs; int64(len(p)) > avail {
			p = p[:avail]
		}
	}
	// No range covers no bytes, so there is nothing to ask for even where the
	// end of the object is still an open question — and nothing worth opening
	// the stream of a reader that has one.
	if len(p) == 0 {
		return 0, nil
	}
	if n, served, err := r.laneReadAt(p, abs); served {
		return n, err
	}

	req, err := r.newRequest(r.ctx)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", abs, abs+int64(len(p))-1))
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
			redactRawURL(r.url), abs, err,
		)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		return 0, r.readUnsatisfied(resp, abs)
	case http.StatusOK:
		// An empty entity is an object of zero bytes rather than a server
		// dropping the range, whatever was asked for and whatever If-Range
		// went with it; see [ReaderAt.Probe]. Held against a size that says
		// otherwise it fails below as the changed object it describes.
		if resp.Header.Get("Content-Range") == "" && emptyEntity(resp) {
			if reason := r.pinOrVerify(resp, 0); reason != "" {
				return 0, fmt.Errorf(
					"%w: reading %s at offset %d: %s",
					ErrObjectChanged, redactRawURL(r.url), abs, reason,
				)
			}
			return 0, io.EOF
		}
		if ifRange != "" {
			// A conditional range answered in full is HTTP saying the validator
			// no longer matches, not that ranges are unsupported.
			return 0, fmt.Errorf(
				"%w: reading %s at offset %d: If-Range did not match",
				ErrObjectChanged, redactRawURL(r.url), abs,
			)
		}
		return 0, fmt.Errorf(
			"%w: reading %s at offset %d",
			ErrRangeIgnored, redactRawURL(r.url), abs,
		)
	default:
		return 0, &StatusCodeError{Code: resp.StatusCode}
	}

	avail, total, err := r.checkPartial(resp, abs, int64(len(p)))
	if err != nil {
		return 0, err
	}
	p = p[:avail]

	n, err := io.ReadFull(resp.Body, p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return n, fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), abs, redactURLError(err),
		)
	}
	// The end of the section ends a read as the end of the object does,
	// whatever the object holds past it.
	if reached := abs + int64(n); reached == total || reached == r.end() {
		return n, io.EOF
	}
	return n, nil
}

// readUnsatisfied takes a 416 for what it says: the request asked past the end
// of the object, which only a read made before the size was settled can do,
// and the complete length reported alongside the refusal settles it. A refusal
// of a range lying inside the object the response itself describes is no
// answer at all, and a 416 saying nothing about the complete length is not
// this case but a status the reader cannot work with.
func (r *ReaderAt) readUnsatisfied(resp *http.Response, off int64) error {
	start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil || start != -1 {
		return &StatusCodeError{Code: resp.StatusCode}
	}
	if reason := r.pinOrVerify(resp, total); reason != "" {
		return fmt.Errorf(
			"%w: reading %s at offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), off, reason,
		)
	}
	if off < total {
		return fmt.Errorf(
			"%w: reading %s at offset %d: range refused within %d bytes",
			ErrObjectChanged, redactRawURL(r.url), off, total,
		)
	}
	return io.EOF
}

// Metadata is collected on the first HTTP response or provided through
// [Config].PriorKnowledge.
// ETag, LastModified and Size are trusted and used to detect a change of the object.
type Metadata struct {
	ETag         string
	LastModified string
	Size         int64
	// Header holds, exactly as they arrived, the headers of the first response
	// the reader accepted.
	Header http.Header
}

// Metadata returns [Metadata] collected during [*ReaderAt.Probe].
//
// As [*ReaderAt.Probe] is called explicitly or lazily by [*ReaderAt.ReadAt],
// the metadata isn't always available.
// The second return value, a boolean flag, is true when Metadata is available.
func (r *ReaderAt) Metadata() (Metadata, bool) {
	m := r.meta.Load()
	if m == nil {
		return Metadata{}, false
	}
	// A response of the reader's own settles the identity whatever it carried,
	// which is more than its validators alone can say: a server sending none
	// leaves them empty and the object pinned all the same.
	identity := m.verified || m.etag != "" || m.lastModified != ""
	return Metadata{
		ETag:         m.etag,
		LastModified: m.lastModified,
		Size:         m.size,
		// Clone of a nil header is nil, which is the reader that has not yet
		// accepted a response saying so.
		Header: m.header.Clone(),
	}, identity && m.sizeKnown
}

func (r *ReaderAt) Close() error {
	// A context.CancelFunc may be called any number of times, which is all
	// Close needs in order to be idempotent.
	//
	// Cancelling first is what keeps Close from waiting: closing the stream
	// waits for the read holding its body to be done with it, and cancelling
	// is what makes that read be done with it.
	r.cancel()
	if r.stream != nil {
		r.stream.kill()
	}
	return nil
}

// checkPartial decides whether a 206 really is the stretch of bytes that was
// asked for, out of the object the reader was built against, before any of its
// body is believed. length is how many bytes the request asked for, after the
// clamp against the end of the object where there was a size to clamp against.
// It reports how many of those bytes the response carries, and the complete
// length it reports them out of.
//
// Ending short of the last byte asked for is only ever the object's own end:
// a read made before the size was settled may ask past it, and the server
// answering the part that exists is the answer. Any other short range is a
// stretch of bytes nobody asked for, and taking it would leave the tail of the
// caller's buffer holding whatever it held before.
func (r *ReaderAt) checkPartial(resp *http.Response, off, length int64) (int64, int64, error) {
	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return 0, 0, fmt.Errorf(
			"httprange: reading %s at offset %d: %w",
			redactRawURL(r.url), off, err,
		)
	}
	wantEnd := off + length - 1
	if start != off || end > wantEnd || (end != wantEnd && end != total-1) {
		return 0, 0, fmt.Errorf(
			"%w: reading %s at offset %d: got bytes %d-%d of %d, want %d-%d",
			ErrObjectChanged, redactRawURL(r.url), off,
			start, end, total, off, wantEnd,
		)
	}

	// Whatever nothing has pinned yet this response gets to say, the complete
	// length among it: with no probe made it is the first word on what the
	// object is, and it completes a description the caller only knew part of.
	if reason := r.pinOrVerify(resp, total); reason != "" {
		return 0, 0, fmt.Errorf(
			"%w: reading %s at offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), off, reason,
		)
	}
	return end - start + 1, total, nil
}

// mismatch says what about resp contradicts m, or "" when nothing does. total
// is the complete length resp amounts to. Each property is compared only where
// both sides state it: a response leaving out its validators, a Doer not
// recording the request it answered, or a description holding nothing yet says
// nothing either way and is not held against the response.
func (m *objectMeta) mismatch(resp *http.Response, total int64) string {
	if m == nil {
		return ""
	}
	if m.sizeKnown && total != m.size {
		return fmt.Sprintf("complete length %d, want %d", total, m.size)
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
