package httprange

import (
	"context"
	"fmt"
	"math"
)

// view is the stretch of the remote object a reader hands out, as absolute
// offsets into that object: [base, base+length). A reader over the whole
// object is the view starting at zero and running to math.MaxInt64, which is
// also how a view ending wherever the object does says so.
type view struct {
	base   int64
	length int64
}

// end returns the absolute offset one past the last byte of the view, or
// math.MaxInt64 where the view runs to the end of the object. Nothing lies
// past that, since no offset does.
func (v view) end() int64 {
	if v.length >= math.MaxInt64-v.base {
		return math.MaxInt64
	}
	return v.base + v.length
}

// locate translates an offset handed to the view into an absolute offset into
// the object, and cuts p down to what lies inside the view. ok is false where
// off is at or past the end of the view, which ends a read however much of the
// object is left.
func (v view) locate(p []byte, off int64) ([]byte, int64, bool) {
	if off >= v.end()-v.base {
		return nil, 0, false
	}
	abs := v.base + off
	if avail := v.end() - abs; int64(len(p)) > avail {
		p = p[:avail]
	}
	return p, abs, true
}

// NewRange is similar to [New] but adds an additional optimization where
// only a single Range Request would be issued for the [off, off+n) range as long as
// the caller reads it sequentially.
//
// Out-of-sequence reads would only disable that optimization. Afterward it is totally the same as
// readers constructed by [New]; each [ReaderAt] issues a Range Request.
func NewRange(ctx context.Context, url string, off, n int64, cfg *Config) (*ReaderAt, error) {
	if off < 0 {
		return nil, fmt.Errorf("httprange.NewRange: negative offset %d", off)
	}
	r, err := New(ctx, url, cfg)
	if err != nil {
		return nil, err
	}
	// The section's reach is computed exactly as io.NewSectionReader computes
	// its limit, wraparound included: off+n overflowing — a negative n lands
	// here, since math.MaxInt64-n wraps below zero — assumes the section may
	// run to the last representable offset.
	var limit int64
	if off <= math.MaxInt64-n {
		limit = off + n
	} else {
		limit = math.MaxInt64
	}
	r.view = view{base: off, length: limit - off}
	if r.length > 0 {
		r.stream = newStream(r.view)
	}
	return r, nil
}
