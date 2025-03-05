package serr

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

var _ error = (*gathered)(nil)
var _ fmt.Formatter = (*gathered)(nil)

type gathered struct{ errs []error }

// Gather wraps errors into single error, removing nil values from errs.
//
// If all errors are nil or len(errs) == 0, Gather returns nil.
//
// The returned error, if not a nil, implements [fmt.Formatter] which prints wrapped errors using given flags and verbs.
// Each error is separated by ", ".
// If the returned error itself should be prefixed with a message, use with [Prefix].
func Gather(errs ...error) error {
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
		return GatherUnchecked(errs...)
	}

	// You would not do filtered := errs[:0] then filtered = append(filtered, nonNilErr)
	// Since it mutates given errs.
	// It would otherwise surprise callers.

	count := 0
	for _, err := range errs {
		if err != nil {
			count++
		}
	}

	idx := 0
	filtered := make([]error, count)
	for _, err := range errs {
		if err != nil {
			filtered[idx] = err
			idx++
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return &gathered{errs: filtered}
}

// GatherChecked is like [Gather] but keeps nil errors in errs.
func GatherChecked(errs ...error) error {
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
	return GatherUnchecked(errs...)
}

// GatherUnchecked is like [Gather] but keeps nil errors and always returns a non-nil error
// when all errs are nil or even len(errs) == 0.
func GatherUnchecked(errs ...error) error {
	return &gathered{errs: errs}
}

type PrefixErr struct {
	P string // Prefix
	E error  // Err
}

// ToPairs converts a prefix-error map into pairs of them sorting keys by cmpKey.
// If cmpKey is nil, [cmp.Compare] will be used.
func ToPairs(errs map[string]error, cmpKey func(i, j string) int) []PrefixErr {
	if len(errs) == 0 {
		return nil
	}
	if cmpKey == nil {
		cmpKey = cmp.Compare
	}
	out := make([]PrefixErr, len(errs))
	idx := 0
	for k, v := range errs {
		out[idx] = PrefixErr{k, v}
		idx++
	}
	slices.SortFunc(out, func(i, j PrefixErr) int { return cmpKey(i.P, j.P) })
	return out
}

// GatherPrefixed is like [GatherChecked] but also errors are prefixed by [Prefix].
//
// If ordering of errs does not matter just build errs by [ToPairs].
func GatherPrefixed(errs []PrefixErr) error {
	// stay using []T
	// other functions using ...error since it is easier to write.
	// But for this case, its []T because you can omit TypeName in slice literal.
	if len(errs) == 0 {
		return nil
	}

	containsNonNil := false
	for _, e := range errs {
		if e.E != nil {
			containsNonNil = true
			break
		}
	}
	if !containsNonNil {
		return nil
	}

	prefixed := make([]error, len(errs))
	for i, prefixErr := range errs {
		prefixed[i] = PrefixUnchecked(prefixErr.P, prefixErr.E)
	}

	return GatherUnchecked(prefixed...)
}

func (e *gathered) Unwrap() []error {
	return e.errs
}

func (e *gathered) format(w io.Writer, fmtStr string) {
	for i, err := range e.errs {
		if i > 0 {
			_, _ = w.Write([]byte(`, `))
		}
		_, _ = fmt.Fprintf(w, fmtStr, err)
	}
}

func (e *gathered) Error() string {
	var s strings.Builder
	e.format(&s, "%s")
	return s.String()
}

// Format implements fmt.Formatter.
//
// Format propagates given flags, width, precision and verb into each error.
// Then it concatenates each result with ", " suffix.
//
// Without Format, e is less readable when printed through fmt.*printf family functions.
// e.g. Format produces lines like
// (%+v) errors, exampleErr: Foo=foo Bar=bar Baz=baz
// (%#v) &errors.errorString{s:"errors"}, &mymodule.exampleErr{Foo:"foo", Bar:"bar", Baz:"baz"}
// instead of (w/o Format)
// (%+v) serr.gathered{(*errors.errorString)(0xc00002c300), (*mymodule.exampleErr)(0xc000102630)}
// (%#v) [824633901824 824634779184]
func (e *gathered) Format(state fmt.State, verb rune) {
	e.format(state, fmt.FormatString(state, verb))
}
