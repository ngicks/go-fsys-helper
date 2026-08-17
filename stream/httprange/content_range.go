package httprange

import (
	"fmt"
	"strconv"
	"strings"
)

// parseContentRange parses a Content-Range header value of the form
// "bytes start-end/total". The unit is matched case-insensitively.
//
// The unsatisfied form "bytes */total", which servers send along a 416
// response (notably for a zero-length object), yields start and end of -1
// and the reported total.
//
// An unknown total ("bytes 0-499/*", "bytes */*"), a unit other than bytes,
// a range not contained in the total, and any syntactically invalid value
// are reported as an error.
func parseContentRange(v string) (start, end, total int64, err error) {
	unit, spec, ok := strings.Cut(strings.TrimSpace(v), " ")
	if !ok || !strings.EqualFold(unit, "bytes") {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: not a bytes range", v)
	}

	rangeSpec, totalSpec, ok := strings.Cut(strings.TrimLeft(spec, " "), "/")
	if !ok {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: no complete length", v)
	}

	total, err = parseDecimal(totalSpec)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: complete length: %w", v, err)
	}

	if rangeSpec == "*" {
		return -1, -1, total, nil
	}

	firstSpec, lastSpec, ok := strings.Cut(rangeSpec, "-")
	if !ok {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: no range", v)
	}
	start, err = parseDecimal(firstSpec)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: first byte pos: %w", v, err)
	}
	end, err = parseDecimal(lastSpec)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: last byte pos: %w", v, err)
	}
	if end < start || end >= total {
		return 0, 0, 0, fmt.Errorf(
			"malformed Content-Range %q: range not within complete length", v,
		)
	}

	return start, end, total, nil
}

// parseDecimal accepts bare decimal digits only; the grammar allows no sign,
// while [strconv.ParseInt] would take "+5" and "-5".
func parseDecimal(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%q is not a decimal number", s)
		}
	}
	return strconv.ParseInt(s, 10, 64)
}
