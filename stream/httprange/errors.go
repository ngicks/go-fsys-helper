package httprange

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrObjectChanged reports that what answered a request is not the object the
// reader was built against: a different length, a different validator, bytes
// other than the ones asked for, or a response from an origin other than the
// one earlier responses settled. The bytes of such a response are never handed
// to the caller, since mixing them with earlier reads would produce an object
// that never existed.
var ErrObjectChanged = errors.New("httprange: remote object changed")

// ErrRangeIgnored reports that the server answered a ranged request with the
// whole entity. Every read would then pull the object down in full while
// looking like a small one, so the reader refuses instead. Construction asks
// nothing of the server, so this surfaces at the first read, at the request a
// [NewRange] reader streams from, or at [ReaderAt.Probe], whichever comes
// first; the error text says which of the three met it.
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
