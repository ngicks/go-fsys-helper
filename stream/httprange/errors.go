package httprange

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrObjectChanged reports that what answered a request is not the object the
// reader was built against: a different length, a different validator, bytes
// other than the ones asked for, or a response from an origin other than the
// one earlier responses settled.
var ErrObjectChanged = errors.New("httprange: remote object changed")

// ErrRangeIgnored reports that the server answered a ranged request with the
// whole entity (status code 200 OK instead of 206 Partial Content).
var ErrRangeIgnored = errors.New("httprange: server ignored range request")

// StatusCodeError reports an unexpected status code.
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
