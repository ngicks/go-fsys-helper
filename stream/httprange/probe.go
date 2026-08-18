package httprange

import (
	"context"
	"fmt"
	"net/http"
)

// Probe checks the reader's picture of the remote object against what the
// server has right now: one ranged GET for a single byte, the cheapest thing
// that both proves the server honours Range and makes it report the complete
// length of the object. Everything the reader already holds — the fields cfg
// carried into [New] among them — is checked against that response and fails
// with an error matching [ErrObjectChanged] where the two disagree; whatever
// was not yet known is pinned from it. A response that is neither that one byte
// nor the refusal an object without it owes fails the same way: bytes from
// somewhere else, more bytes than were asked for, and a refusal reporting an
// object long enough to hold that byte are all a server not honouring Range. A
// server handing back the whole entity fails with an error matching
// [ErrRangeIgnored], instead of letting every later read quietly download the
// object in full. The single exception is a whole entity of zero bytes, which is
// read as an object of size zero: that is how an empty one answers a range
// request on many servers, Go's own file server among them.
//
// Until some request happens, what the caller handed in is trusted rather than
// verified, and how large the object is may be unknown. Probe is the way to
// put that verification, and the failure it may turn into, ahead of the first
// byte. Skipping it costs nothing: the same check runs inside whichever
// request the reader makes first, over that request's own response, so the
// lazy path never spends a round trip of its own. Calling Probe on a reader
// whose object is already verified fires all the same and verifies it again.
//
// ctx bounds this call alone, where the context given to [New] bounds the
// reader; the request obeys both and fails as soon as either is done.
func (r *ReaderAt) Probe(ctx context.Context) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The reader's own context is the other half of the bound, and this is
	// what makes the request obey it without becoming its child: cancel runs
	// as soon as either context is done.
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()

	return r.probe(reqCtx)
}

// probe is the request half of [ReaderAt.Probe]: it asks for one byte, reads
// the complete length out of whatever answer comes back, and hands the
// response to [ReaderAt.pinOrVerify], which is the half every read shares.
func (r *ReaderAt) probe(ctx context.Context) error {
	req, err := r.newRequest(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	// No If-Range, though the reader may well have a validator to send: a
	// changed object would answer a conditional range with the whole entity,
	// which is both a download nobody asked for and a poorer account of what
	// happened than the mismatch the response's own validators tell.

	resp, err := r.client.Do(req)
	if err != nil {
		return redactURLError(err)
	}
	defer drainAndClose(resp.Body)

	if err := checkContentEncoding(resp); err != nil {
		return fmt.Errorf("httprange: probing %s: %w", redactRawURL(r.url), err)
	}

	var total int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, end, reported, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("httprange: probing %s: %w", redactRawURL(r.url), err)
		}
		if start != 0 || end != 0 {
			return fmt.Errorf(
				"%w: probing %s: got bytes %d-%d of %d, want 0-0",
				ErrObjectChanged, redactRawURL(r.url), start, end, reported,
			)
		}
		total = reported
	case http.StatusRequestedRangeNotSatisfiable:
		return r.probeUnsatisfied(resp)
	case http.StatusOK:
		// An empty body is the one whole-entity answer worth taking, and it is
		// a common one: Go's own http.ServeContent replies 200 rather than 416
		// to a range request on an empty file. Nothing is lost by accepting it,
		// since a body of zero bytes is the entire representation however it
		// was asked for, and a reader of size zero never asks for a range
		// again. Any other 200 really would drag the whole object down on
		// every read.
		if resp.Header.Get("Content-Range") != "" || !emptyEntity(resp) {
			return fmt.Errorf(
				"%w: probing %s",
				ErrRangeIgnored, redactRawURL(r.url),
			)
		}
		total = 0
	default:
		return &StatusCodeError{Code: resp.StatusCode}
	}

	if reason := r.pinOrVerify(resp, total); reason != "" {
		return fmt.Errorf(
			"%w: probing %s: %s",
			ErrObjectChanged, redactRawURL(r.url), reason,
		)
	}
	return nil
}

// probeUnsatisfied takes a 416 answering the probe the way
// [ReaderAt.readUnsatisfied] takes one answering a read: an empty object has no
// first byte to hand over, so this is how it refuses a request for one, and the
// complete length reported alongside the refusal is exactly what the probe came
// for. A refusal of a byte lying inside the object the response itself
// describes is no answer at all, and a 416 saying nothing about the complete
// length is not this case but a status the reader cannot work with.
func (r *ReaderAt) probeUnsatisfied(resp *http.Response) error {
	start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil || start != -1 {
		return &StatusCodeError{Code: resp.StatusCode}
	}
	if reason := r.pinOrVerify(resp, total); reason != "" {
		return fmt.Errorf(
			"%w: probing %s: %s",
			ErrObjectChanged, redactRawURL(r.url), reason,
		)
	}
	if total > 0 {
		return fmt.Errorf(
			"%w: probing %s: range refused within %d bytes",
			ErrObjectChanged, redactRawURL(r.url), total,
		)
	}
	return nil
}
