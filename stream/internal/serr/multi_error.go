package serr

import (
	"bytes"
	"fmt"
	"sync"
)

var bufPool = &sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func getBuf() *bytes.Buffer {
	return bufPool.Get().(*bytes.Buffer)
}

func putBuf(b *bytes.Buffer) {
	if b.Cap() > 64*1024 {
		// See https://golang.org/issue/23199
		return
	}
	b.Reset()
	bufPool.Put(b)
}

var _ error = (*multiError)(nil)
var _ fmt.Formatter = (*multiError)(nil)

type multiError struct{ errs []error }

// NewMultiError wraps errors into single error, ignoring nil values in errs.
//
// If all errors are nil or len(errs) == 0, NewMultiError returns nil.
//
// errs is retained by returned error.
// Callers should not mutate errs after NewMultiErrorChecked returns.
func NewMultiError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	var i int
	for i = 0; i < len(errs); i++ {
		if errs[i] == nil {
			break
		}
	}
	if i == len(errs) {
		return NewMultiErrorUnchecked(errs)
	}
	var filtered []error
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return &multiError{errs: filtered}
}

// NewMultiErrorChecked wraps errors into single error if and only if errs contains at least one non nil error.
// It also preserves nil errors in errs for better printing.
// This is useful when an error itself does not contain information
// to pin-point how and why error is caused other than just index within error slice.
//
// NewMultiErrorChecked returns nil if len(errs) == 0 or all errors are nil.
//
// errs is retained by returned error.
// Callers should not mutate errs after NewMultiErrorChecked returns.
func NewMultiErrorChecked(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	containsNonNil := false
	for _, e := range errs {
		if e != nil {
			containsNonNil = true
			break
		}
	}
	if !containsNonNil {
		return nil
	}
	return NewMultiErrorUnchecked(errs)
}

// NewMultiErrorUnchecked wraps errors into single error.
// As suffix "unchecked" implies it does not do any filtering for errs.
// The returned error is always non nil even if all errors are nil or len(errs) == 0.
//
// errs is retained by returned error.
// Callers should not mutate errs after NewMultiErrorChecked returns.
func NewMultiErrorUnchecked(errs []error) error {
	return &multiError{errs: errs}
}

func (me *multiError) str(fmtStr string) string {
	if len(me.errs) == 0 {
		return "MultiError: "
	}

	buf := getBuf()
	defer putBuf(buf)

	_, _ = buf.WriteString("MultiError: ")

	for _, e := range me.errs {
		_, _ = fmt.Fprintf(buf, fmtStr, e)
		_, _ = buf.WriteString(", ")
	}

	// This line is safe since:
	// For cases where len(me.errs) == 0, it removes `: ` suffix.
	// For other cases it removes `, ` suffix.
	buf.Truncate(buf.Len() - 2)

	return buf.String()
}

func (me *multiError) Error() string {
	return me.str("%s")
}

func (me *multiError) Unwrap() []error {
	return me.errs
}

// Format implements fmt.Formatter.
//
// Format propagates given flags, width, precision and verb into each error.
// Then it concatenates each result with ", " suffix.
//
// Without Format, me is less readable when printed through fmt.*printf family functions.
// e.g. Format produces lines like
// (%+v) MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz
// (%#v) MultiError: &errors.errorString{s:"errors"}, &mymodule.exampleErr{Foo:"foo", Bar:"bar", Baz:"baz"}
// instead of (w/o Format)
// (%+v) stream.multiError{(*errors.errorString)(0xc00002c300), (*mymodule.exampleErr)(0xc000102630)}
// (%#v) [824633901824 824634779184]
func (me *multiError) Format(state fmt.State, verb rune) {
	state.Write([]byte(me.str(fmt.FormatString(state, verb))))
}
