package httprange

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

// drainLimit caps what [drainAndClose] reads out of a response body.
const drainLimit = 1 << 16

// Doer is a direct interface translation of [*http.Client].
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Config struct {
	// Client issues every request the reader makes. nil means
	// http.DefaultClient.
	Client Doer
	// Additional headers put on every HTTP request.
	// Use this to include any metadata, for example authorization info.
	//
	// Range, If-Range and Accept-Encoding will be deliberately overwritten.
	Header http.Header
	// PriorKnowledge provides trusted prior knowledge for an HTTP object of interest.
	// Partial metadata is ok: you need to fill only known fields.
	//
	// For example, you can provide ETag and/or LastModified to detect content change
	// between 2 or more download attempts. Useful for resumable downloads.
	//
	// ReaderAt.Metadata returns fully filled info collected while accessing the URL.
	// Resuming download may use the method and saved info with this field.
	//
	// Metadata.Header is deliberately dropped before being used; you don't want to fill it.
	PriorKnowledge Metadata
}

// ReaderAt reads a remote HTTP object through range requests.
// This type only implements [io.ReaderAt]; use this with [io.SectionReader] if a consumer
// only accepts [io.Reader].
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

	// view is the stretch of the object this reader hands out, and what the
	// offsets its caller reads at are relative to. A reader over the whole
	// object is the view running from zero to wherever the object ends.
	view

	// stream is the one range request serving the reads that arrive in order,
	// for a caller who said up front which stretch of the object they would
	// read that way. nil — what [New] builds, and what a view with nothing in
	// it gets — is a reader whose every read is a bounded request of its own.
	stream *stream
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
	// header is the whole of what the first accepted response carried, held
	// for the caller who wants the entity headers of the object out of it. It
	// is nothing later responses are checked against — which object this is,
	// the validators say — and it settles once, like every other property
	// here. nil is the case of no response yet.
	header http.Header
	// verified says whether a response has been through pinOrVerify, which is
	// what tells a description the caller vouched for apart from one the
	// server has confirmed. It sits in the description rather than beside it so
	// that the two settle in one step: a flag of its own would leave a moment
	// where a reader finds what a response pinned and nothing saying a response
	// pinned it.
	verified bool
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
		verified:     true,
		// A copy, so that what gets pinned is detached from the response it
		// came off and stands however the response is disposed of afterwards.
		header: resp.Header.Clone(),
	}
}

// completedBy returns m with every property it leaves unstated taken from o,
// or m itself when there was nothing to fill in. A nil m holds nothing, so o
// completes all of it.
func (m *objectMeta) completedBy(o *objectMeta) *objectMeta {
	if m == nil {
		return o
	}
	// Whether anything was actually taken from o is tracked as it happens
	// rather than compared for afterwards: a description holds the response
	// headers, and a struct holding a map is not comparable.
	filled := *m
	var took bool
	if filled.etag == "" && o.etag != "" {
		filled.etag, took = o.etag, true
	}
	if filled.lastModified == "" && o.lastModified != "" {
		filled.lastModified, took = o.lastModified, true
	}
	if filled.origin == "" && o.origin != "" {
		filled.origin, took = o.origin, true
	}
	if !filled.sizeKnown && o.sizeKnown {
		filled.size, filled.sizeKnown, took = o.size, true, true
	}
	if !filled.verified && o.verified {
		filled.verified, took = true, true
	}
	if filled.header == nil && o.header != nil {
		filled.header, took = o.header, true
	}
	if !took {
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

// New returns a [*ReaderAt] over url.
// cfg is ok to be nil. In that case cfg is treated as a zero value.
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
		view:   view{length: math.MaxInt64},
	}
	known := cfg.PriorKnowledge
	if known.ETag != "" || known.LastModified != "" || known.Size > 0 {
		// The origin stays open: which server answers is nothing the caller can
		// have saved, and nothing is known about it until a response arrives.
		//
		// The headers stay open for a reason of their own, and this listing the
		// fields one by one rather than copying the struct is what keeps them
		// that way: a snapshot handed back carries the headers of some earlier
		// reader's response, which describe that response and not this one.
		// Header on the way in is ignored, and a header the caller can read
		// back off this reader is one this reader was answered with.
		r.meta.Store(&objectMeta{
			etag:         known.ETag,
			lastModified: known.LastModified,
			size:         known.Size,
			sizeKnown:    known.Size > 0,
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

// emptyEntity reports whether a 200 carrying no Content-Range is an entity of
// zero bytes. A response that states its length says so directly; one that
// does not — chunked, most often — is read for a single byte, an immediate
// EOF being the emptiness and anything else the whole entity the status
// already suggested.
func emptyEntity(resp *http.Response) bool {
	switch resp.ContentLength {
	case 0:
		return true
	case -1:
		var b [1]byte
		n, err := resp.Body.Read(b[:])
		return n == 0 && errors.Is(err, io.EOF)
	default:
		return false
	}
}
