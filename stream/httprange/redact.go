// Package httprange reads remote objects through HTTP range requests.
package httprange

import (
	"errors"
	"net/url"
)

// redactedURLPlaceholder stands in for a URL that could not be parsed.
// Echoing the raw text back would defeat redaction, since an unparsable URL
// can still carry a credential.
const redactedURLPlaceholder = "[redacted url]"

// redactURL renders u as scheme, host and path only, dropping userinfo,
// query and fragment.
func redactURL(u *url.URL) string {
	redacted := *u
	redacted.User = nil
	redacted.RawQuery = ""
	redacted.ForceQuery = false
	redacted.Fragment = ""
	redacted.RawFragment = ""
	return redacted.String()
}

// redactRawURL parses raw and applies [redactURL] to it.
func redactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return redactedURLPlaceholder
	}
	return redactURL(u)
}

// redactURLError replaces the URL carried by a [url.Error] anywhere in err's
// tree with its redacted form, so that presigned tokens, basic-auth
// passwords and the like never reach error text.
//
// The underlying Err is carried over untouched, keeping [errors.Is] and
// [errors.As] working through the result. Any error not built around a
// [url.Error] is returned as is.
func redactURLError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	return &url.Error{
		Op:  ue.Op,
		URL: redactRawURL(ue.URL),
		Err: ue.Err,
	}
}
