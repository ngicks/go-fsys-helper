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
//
// A transport error reaches the caller with the URL it names replaced by
// scheme, host and path alone, which the reader can do for an error that is or
// wraps a [*url.Error]. A Doer describing a failure in words of its own is
// beyond that reach: whatever it writes into its message, the raw URL
// included, is passed on as it stands.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config describes how a [ReaderAt] talks to the remote object, and what the
// caller already knows about that object. The zero value is usable: requests
// go through [http.DefaultClient] carrying no extra headers, and everything
// about the object is learned from the first response.
//
// Size, ETag and LastModified are metadata rather than settings, and any
// subset of them may be given. They are trusted, not verified, until a request
// actually happens: the first response is checked against every one of them
// and contradicts them with [ErrObjectChanged] before any of its bytes are
// used. None of them decides when a request happens; see [ReaderAt.Probe] for
// putting that check ahead of the first read.
type Config struct {
	// Client issues every request the reader makes. nil means
	// [http.DefaultClient].
	Client Doer
	// Header holds headers to put on every request, such as an Authorization
	// line or an API version. Whatever it carries stays out of error text.
	//
	// Range, If-Range and Accept-Encoding decide what a response means, so the
	// reader owns them and no value handed in for one of the three reaches the
	// wire: the first two are dropped from the copy that goes out and
	// Accept-Encoding is overwritten with identity.
	Header http.Header
	// Size is the total size of the object in bytes. Greater than zero means
	// the caller already knows it, which spares the reader the request that
	// would otherwise settle it. Zero, the usual case, leaves the size to the
	// first response that reports it.
	Size int64
	// ETag and LastModified are validators the caller saved from an earlier
	// response, for resuming. They pre-pin the object identity and ride every
	// request's If-Range, subject to the same strong-validator rule as
	// validators the reader pinned itself. Empty means none.
	ETag         string
	LastModified string
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
// whether this was caught by a probe or by a read.
//
// An entity of zero bytes is not reported this way; it is read as an object of
// size zero. See [ReaderAt.Probe].
var ErrRangeIgnored = errors.New("httprange: server ignored range request")

// StatusCodeError reports a response status the reader cannot work with.
// [errors.AsType] pulls one out of what a call returns.
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
// own, and the description of the object those requests share settles one
// property at a time, once, by whichever of them gets there first.
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

	// meta says which object the reader is reading and how large it is.
	// Whatever the caller handed in seeds it, and whatever that leaves open
	// the first response to arrive fills in. Several goroutines may reach that
	// at once, hence the atomic. Each property goes from unstated to settled
	// once and is what later responses are held to from then on.
	meta atomic.Pointer[objectMeta]
	// verified says whether a response has been through pinOrVerify yet, which
	// is what tells a description the caller vouched for apart from one the
	// server has confirmed.
	verified atomic.Bool
}

// objectMeta is what later responses get checked against: the validators the
// object is known by, the origin its requests land on and its total size. A
// property is unstated while neither the caller nor any response so far has
// said what it holds.
type objectMeta struct {
	etag         string
	lastModified string
	origin       string
	// size is the total size of the object in bytes, meaningful only once
	// sizeKnown says something has stated it. Zero cannot stand in for the
	// unstated case: an object of zero bytes has a size of zero.
	size      int64
	sizeKnown bool
}

// metaFromResponse is what resp says about the object, total being the
// complete length its status line and headers amount to.
func metaFromResponse(resp *http.Response, total int64) *objectMeta {
	return &objectMeta{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		origin:       originOf(resp),
		size:         total,
		sizeKnown:    true,
	}
}

// completedBy returns m with every property it leaves unstated taken from o,
// or m itself when there was nothing to fill in. A nil m holds nothing, so o
// completes all of it.
func (m *objectMeta) completedBy(o *objectMeta) *objectMeta {
	if m == nil {
		return o
	}
	filled := *m
	if filled.etag == "" {
		filled.etag = o.etag
	}
	if filled.lastModified == "" {
		filled.lastModified = o.lastModified
	}
	if filled.origin == "" {
		filled.origin = o.origin
	}
	if !filled.sizeKnown {
		filled.size, filled.sizeKnown = o.size, o.sizeKnown
	}
	if filled == *m {
		return m
	}
	return &filled
}

// pinOrVerify folds what resp says about the object into what the reader has
// pinned, and reports what of the two contradicts the other, or "" when
// nothing does. total is the complete length the response amounts to. Every
// property is taken on its own: one nothing has pinned yet is pinned to what
// the response carries, and one already pinned stands, so a response
// disagreeing with it is describing another object.
//
// This is the whole of what a probe does with a response, and every request
// the reader makes puts its own response through it, which is why the lazy
// probe costs nothing beyond the read it rides on.
//
// Several goroutines can be doing this to the same half-filled description at
// once, so a lost race is retried against what the winner pinned rather than
// overwriting it.
func (r *ReaderAt) pinOrVerify(resp *http.Response, total int64) string {
	seen := metaFromResponse(resp, total)
	for {
		pinned := r.meta.Load()
		if reason := pinned.mismatch(resp, total); reason != "" {
			return reason
		}
		filled := pinned.completedBy(seen)
		if filled == pinned || r.meta.CompareAndSwap(pinned, filled) {
			r.verified.Store(true)
			return ""
		}
	}
}

// knownSize reports the total size of the object and whether anything has
// stated it yet.
func (r *ReaderAt) knownSize() (int64, bool) {
	m := r.meta.Load()
	if m == nil {
		return 0, false
	}
	return m.size, m.sizeKnown
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
// ctx bounds every request the reader will ever make: once it is done the
// reader is too. cfg may be nil, which means a zero [Config].
//
// Nothing goes on the wire here. Whatever cfg carries — a size, validators,
// any subset of them, nothing at all — is what the reader starts out believing
// about the object, and the first request it makes is the first chance to find
// out whether that is true. A server that does not honour Range, a status the
// reader cannot work with, and an object other than the one cfg describes all
// surface at the first read, or at [ReaderAt.Probe] for a caller who wants to
// know before any byte moves.
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
	if cfg.ETag != "" || cfg.LastModified != "" || cfg.Size > 0 {
		// The origin stays open: which server answers is nothing the caller can
		// have saved, and nothing is known about it until a response arrives.
		r.meta.Store(&objectMeta{
			etag:         cfg.ETag,
			lastModified: cfg.LastModified,
			size:         cfg.Size,
			sizeKnown:    cfg.Size > 0,
		})
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
func (r *ReaderAt) newRequest(ctx context.Context) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
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
