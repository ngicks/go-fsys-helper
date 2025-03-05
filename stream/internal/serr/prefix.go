package serr

import (
	"fmt"
	"io"
	"strings"
)

var (
	_ error         = (*prefixed)(nil)
	_ fmt.Formatter = (*prefixed)(nil)
)

type prefixed struct {
	prefix string
	err    error
}

// Prefix prefixes err with prefix.
// It returns nil if err is nil.
//
// The returned error, if not a nil, implements [fmt.Formatter]
// which prints prefix and the wrapped error using given flags and verbs.
func Prefix(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return PrefixUnchecked(prefix, err)
}

// PrefixUnchecked is like [Prefix] but always returned non nil error.
func PrefixUnchecked(prefix string, err error) error {
	return &prefixed{
		prefix: prefix,
		err:    err,
	}
}

func (e *prefixed) Unwrap() error {
	return e.err
}

func (e *prefixed) format(w io.Writer, format string) {
	_, _ = io.WriteString(w, e.prefix)
	_, _ = fmt.Fprintf(w, format, e.err)
}

func (e *prefixed) Error() string {
	var s strings.Builder
	e.format(&s, "%s")
	return s.String()
}

func (e *prefixed) Format(state fmt.State, verb rune) {
	e.format(state, fmt.FormatString(state, verb))
}
