package httprange

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

// drainLimit caps what [drainAndClose] reads out of a response body.
const drainLimit = 1 << 16

// Doer issues an HTTP request and returns its response. [http.Client]
// satisfies it, and so does anything wrapping one with retries, request
// signing or instrumentation.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config describes how a [ReaderAt] talks to the remote object. The zero value
// is usable: requests go through [http.DefaultClient] carrying no extra
// headers, and the size of the object is discovered by probing it.
type Config struct {
	// Client issues every request the reader makes. nil means
	// [http.DefaultClient].
	Client Doer
	// Header holds headers to put on every request, such as an Authorization
	// line or an API version. Whatever it carries stays out of error text.
	//
	// Range, If-Range and Accept-Encoding are removed from the copy that goes
	// out: those decide what a response means, so the reader owns them.
	Header http.Header
	// Size is the total size of the object in bytes. Greater than zero means
	// the caller already knows it and vouches for it, which saves the reader a
	// round trip at construction but leaves a server that does not support
	// ranges to be found out by the first read. Zero, the usual case, asks the
	// reader to go and find out.
	Size int64
}

// ErrObjectChanged reports that what answered a read is not the object the
// reader was built against: a different length, a different validator, bytes
// from an offset other than the one asked for, or a response from an origin
// other than the one construction settled on. The bytes of such a response are
// never handed to the caller, since mixing them with earlier reads would
// produce an object that never existed.
var ErrObjectChanged = errors.New("httprange: remote object changed")

// ErrRangeIgnored reports that the server answered a ranged request with the
// whole entity. Every read would then pull the object down in full while
// looking like a small one, so the reader refuses instead. The error text says
// whether this was caught by the probe at construction or by a later read.
//
// An entity of zero bytes is not reported this way; the probe reads it as an
// object of size zero. See [New].
var ErrRangeIgnored = errors.New("httprange: server ignored range request")

// StatusCodeError reports a response status the reader cannot work with.
//
// It deliberately does not wrap [io/fs.ErrNotExist] for 404 and 410. A remote
// HTTP object is not a file: those statuses can just as well come from a proxy
// in the way, an expired signature or a routing mistake, and code branching on
// fs.ErrNotExist would read all of them as "the file is not there" and, say,
// create it. Callers who want that meaning ask for it through NotFound.
type StatusCodeError struct {
	Code int
}

func (e *StatusCodeError) Error() string {
	if text := http.StatusText(e.Code); text != "" {
		return fmt.Sprintf("httprange: unexpected status %d %s", e.Code, text)
	}
	return fmt.Sprintf("httprange: unexpected status %d", e.Code)
}

// NotFound reports whether the status says the object is not there:
// 404 Not Found or 410 Gone.
func (e *StatusCodeError) NotFound() bool {
	return e.Code == http.StatusNotFound || e.Code == http.StatusGone
}

// ReaderAt reads a remote HTTP object through range requests. Build one with
// [New].
//
// It is safe for concurrent use: each [ReaderAt.ReadAt] runs a request of its
// own and everything it consults is settled once and only read afterwards.
type ReaderAt struct {
	client Doer
	url    string
	header http.Header

	// ctx bounds every request this reader will ever make, and Close cancels
	// it. A context normally travels as a parameter, but io.ReaderAt.ReadAt
	// has nowhere to put one, so it rides along for the lifetime of the value
	// the way http.Request carries one.
	ctx    context.Context
	cancel context.CancelFunc

	size int64

	// meta says which object the reader is reading. The probe fills it in; when
	// the probe was skipped the first successful ranged response does instead,
	// which several goroutines may reach at once, hence the atomic. Nothing
	// rewrites it once set.
	meta atomic.Pointer[objectMeta]
}

// objectMeta is what later responses get checked against: the validators the
// object was first seen with and the origin its requests landed on. A field is
// empty when the response did not carry it.
type objectMeta struct {
	etag         string
	lastModified string
	origin       string
}

func metaFromResponse(resp *http.Response) *objectMeta {
	return &objectMeta{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		origin:       originOf(resp),
	}
}

// originOf returns the scheme and host the response actually came from, which
// is the far end of any redirect chain rather than the URL that was asked for.
// It is empty when the Doer did not record the request it answered, in which
// case that Doer opts out of origin pinning.
func originOf(resp *http.Response) string {
	if resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	return resp.Request.URL.Scheme + "://" + resp.Request.URL.Host
}

// New returns a [ReaderAt] over url.
//
// ctx bounds every request the reader will ever make, not merely the ones New
// itself issues: once it is done the reader is too. cfg may be nil, which
// means a zero [Config].
//
// Unless cfg.Size already says how large the object is, New spends one small
// ranged GET on learning that, on proving that the server honours Range at
// all, and on noting the validators and the post-redirect origin it answered
// with. A server handing back the whole entity fails here with an error
// matching [ErrRangeIgnored], instead of letting every later read quietly
// download the object in full. The single exception is a whole entity of zero
// bytes, which New takes as an object of size zero: that is how an empty one
// answers a range request on many servers, Go's own file server among them.
//
// With cfg.Size given there is no such request, so a server that does not do
// ranges is found out by the first read, and the validators and origin are
// pinned by the first response that succeeds.
func New(ctx context.Context, url string, cfg *Config) (*ReaderAt, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if err := validateURL(url); err != nil {
		return nil, err
	}

	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	header := cfg.Header.Clone()
	if header == nil {
		header = http.Header{}
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	r := &ReaderAt{
		client: client,
		url:    url,
		header: header,
		ctx:    sessionCtx,
		cancel: cancel,
	}
	if cfg.Size > 0 {
		r.size = cfg.Size
		return r, nil
	}
	if err := r.probe(); err != nil {
		cancel()
		return nil, err
	}
	return r, nil
}

// validateURL rejects what can never become a ranged GET, so that a typo or a
// URL of the wrong kind surfaces at construction rather than on a read.
func validateURL(raw string) error {
	if raw == "" {
		return errors.New("httprange: empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("httprange: %w", redactURLError(err))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf(
			"httprange: url %s: scheme %q is not http or https",
			redactURL(u), u.Scheme,
		)
	}
	if u.Host == "" {
		return fmt.Errorf("httprange: url %s: no host", redactURL(u))
	}
	return nil
}

// newRequest builds the GET that both the probe and a read start from: the
// configured static headers go on first and the headers deciding what the
// response means go on after, so a stray Range or If-Range handed in through
// the configuration cannot survive into the wire request.
func (r *ReaderAt) newRequest() (*http.Request, error) {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, redactURLError(err)
	}
	req.Header = r.header.Clone()
	req.Header.Del("Range")
	req.Header.Del("If-Range")
	// Go's transport already leaves its automatic gzip off for a ranged
	// request, but a Doer supplied by the caller need not, and bytes that
	// arrived compressed have nothing to do with the offsets asked for.
	req.Header.Set("Accept-Encoding", "identity")
	return req, nil
}

// probe issues the single request New makes: a ranged GET for one byte, the
// cheapest thing that both proves the server honours Range and makes it report
// the complete length of the object.
func (r *ReaderAt) probe() error {
	req, err := r.newRequest()
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := r.client.Do(req)
	if err != nil {
		return redactURLError(err)
	}
	defer drainAndClose(resp.Body)

	if err := checkContentEncoding(resp); err != nil {
		return fmt.Errorf("httprange: probing %s: %w", redactRawURL(r.url), err)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("httprange: probing %s: %w", redactRawURL(r.url), err)
		}
		if start != 0 {
			return fmt.Errorf(
				"httprange: probing %s: asked for byte 0, got bytes from %d",
				redactRawURL(r.url), start,
			)
		}
		r.size = total
	case http.StatusRequestedRangeNotSatisfiable:
		// An empty object has no first byte to hand over, so this is how it
		// answers the probe. The complete length it has to report alongside the
		// refusal is exactly what the probe came for.
		start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("httprange: probing %s: %w", redactRawURL(r.url), err)
		}
		if start != -1 {
			return fmt.Errorf(
				"httprange: probing %s: status 416 without an unsatisfied Content-Range",
				redactRawURL(r.url),
			)
		}
		r.size = total
	case http.StatusOK:
		// An empty body is the one whole-entity answer worth taking, and it is
		// a common one: Go's own http.ServeContent replies 200 rather than 416
		// to a range request on an empty file. Nothing is lost by accepting it,
		// since a body of zero bytes is the entire representation however it
		// was asked for, and a reader of size zero never asks for a range
		// again. Any other 200 really would drag the whole object down on
		// every read.
		if resp.Header.Get("Content-Range") != "" || resp.ContentLength != 0 {
			return fmt.Errorf(
				"%w: probing %s at construction",
				ErrRangeIgnored, redactRawURL(r.url),
			)
		}
		r.size = 0
	default:
		return &StatusCodeError{Code: resp.StatusCode}
	}

	r.meta.Store(metaFromResponse(resp))
	return nil
}

// checkContentEncoding refuses a body that did not arrive as the object's own
// bytes. Under a compressing encoding the offsets and lengths on the wire say
// nothing about the offsets and lengths that were asked for, so no range
// arithmetic over such a response can be trusted.
func checkContentEncoding(resp *http.Response) error {
	if resp.Uncompressed {
		return errors.New("response body was decompressed in transit")
	}
	enc := resp.Header.Get("Content-Encoding")
	if enc == "" || strings.EqualFold(enc, "identity") {
		return nil
	}
	return fmt.Errorf("response Content-Encoding %q is not identity", enc)
}

// drainAndClose reads off what the caller left behind before closing, so that
// the connection underneath can go back to the pool. A body far longer than
// the request asked for is abandoned at drainLimit instead: reading it out
// would cost more than keeping the connection is worth.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.CopyN(io.Discard, body, drainLimit)
	_ = body.Close()
}
