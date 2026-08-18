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

// NewRange returns a [ReaderAt] over the section of the remote object that
// starts at off and runs for n bytes, the way [io.NewSectionReader] does over
// a local one: the offsets handed to [ReaderAt.ReadAt] are relative to
// off, and a read reaching off+n ends at [io.EOF] whatever the object holds
// past there. An n longer than what is left clamps to the end of the object,
// and math.MaxInt64 is how a caller says "from off to wherever the object
// ends". An n of zero is a section with nothing in it: the first read is
// io.EOF and no request is ever made. The section's reach is computed as
// io.NewSectionReader computes it, so an off+n that overflows — a negative n
// among the ways there — runs to the end the same way math.MaxInt64 does.
//
// It is [New] with one thing added, for the caller who says up front that they
// will read the section mostly front to back: a single streaming range request
// serves the reads arriving in order, so a scan costs one round trip rather
// than one per buffer.
//
//	r, err := httprange.NewRange(ctx, url, 0, math.MaxInt64, nil)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	_, err = io.Copy(dst, io.NewSectionReader(r, 0, math.MaxInt64))
//
// A read arriving anywhere but where that stream stands closes it for good,
// and every read from then on is a bounded request of its own, exactly as
// under New; so does a stream that failed halfway, whose failure is the
// caller's to retry on terms of their own. Nothing re-opens it: a caller with
// a second stretch to walk builds a second reader.
//
// Nothing goes on the wire here, whatever cfg carries. The stream opens on the
// first read it can serve and its response carries the check [ReaderAt.Probe]
// describes, so a section of an object that changed, or that was never there,
// surfaces at that read — or at Probe, for a caller who wants to know before
// any byte moves.
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
