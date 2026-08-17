package httprange

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
)

// streamState says how far the lane has got. It only ever moves forward:
// pending until a request opens it, open while a body is handing bytes over,
// and dead from the first thing the lane cannot do. Nothing brings a dead lane
// back. A stream that failed is a connection someone may want to try again on
// terms of their own — how many times, how long after, against which of
// several mirrors — and none of that is anything to decide behind their back.
type streamState uint8

const (
	// streamPending is a lane a caller declared and nothing has opened yet: it
	// holds no more than the stretch of the object it was declared for.
	streamPending streamState = iota
	// streamOpen is a lane holding the body of its one response, whose next
	// byte belongs at pos.
	streamOpen
	// streamDead is a lane that will serve nothing further, whether it was
	// read to its end, failed halfway, or had nothing to serve to begin with.
	streamDead
)

// stream is the single range request behind a view whose caller said up front
// they would read it mostly front to back: one response, whose body serves the
// reads arriving in order, and never a second one.
//
// A read at any other offset is served by a bounded request of its own, just
// as every read is on a reader with no lane, and it must not pay for the
// lane's existence: pos alone is what tells it so, which is why pos is atomic
// and why mu is taken only by a read the lane is actually going to serve.
type stream struct {
	// mu guards body, state and limit, and serializes the one open.
	mu    sync.Mutex
	body  io.ReadCloser
	state streamState
	// limit is the absolute offset one past the last byte the response
	// granted, which is where its body ends. It says nothing until the lane is
	// open.
	limit int64

	// pos is the absolute offset the body's next byte belongs at. It is atomic
	// so that a read at some other offset can find that out without taking mu
	// and queueing behind whatever the network is doing. A read that does
	// match takes mu and looks again: another goroutine may have taken the
	// position in between, and a position on its own says nothing about the
	// lane still being alive.
	pos atomic.Int64

	// base and length are the view the lane was declared for, as absolute
	// offsets into the object, and never change once it is built. length is
	// math.MaxInt64 for a view running to the end of the object, wherever that
	// turns out to be, and is otherwise greater than zero: a view with nothing
	// in it has nothing to stream and gets no lane at all.
	base   int64
	length int64
}

// newStream returns the lane for the view [base, base+length), pending until
// the first read it can serve opens it.
func newStream(base, length int64) *stream {
	s := &stream{base: base, length: length}
	s.pos.Store(base)
	return s
}

// end returns the absolute offset one past the last byte of the view, or
// math.MaxInt64 where the view runs to the end of the object.
func (s *stream) end() int64 {
	if s.length >= math.MaxInt64-s.base {
		return math.MaxInt64
	}
	return s.base + s.length
}

// kill closes the lane for good, handing the connection underneath back where
// there is enough left of the body to hand it back. It may be called any
// number of times and from anywhere, which is what makes it the single way a
// lane ends, whether that is a read out of order, a body that failed, or the
// reader closing.
func (s *stream) kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.killLocked()
}

// killLocked is [stream.kill] for a caller already holding mu.
func (s *stream) killLocked() {
	if s.state == streamOpen {
		drainAndClose(s.body)
		s.body = nil
	}
	s.state = streamDead
}

// streamReadAt serves p out of the lane's body, off being the absolute offset
// the caller is reading at.
//
// served says whether the lane answered at all: false means it is not the one
// holding off — it died, or another read took the position first — and the
// caller has to make a bounded request of its own, n and err saying nothing.
// A lane still pending opens here, on the first read it can serve, and an
// answer the open-time gate refuses comes back as that error rather than as a
// fall-back: the same response would be refused whichever request had carried
// it.
//
// The only [io.EOF] it ever returns means the lane has nothing further to
// hand over — the body reached the end of the view or the end of the object,
// or there was never anything there to stream — and the lane is dead by then.
// A body failing or ending short comes back instead as the bytes read so far
// plus the error, [io.ErrUnexpectedEOF] for the short case, for the caller to
// put its own words around.
func (r *ReaderAt) streamReadAt(p []byte, off int64) (n int, served bool, err error) {
	s := r.stream

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == streamDead || s.pos.Load() != off {
		return 0, false, nil
	}
	if s.state == streamPending {
		if err := r.openStream(); err != nil {
			return 0, true, err
		}
	}

	for {
		remaining := s.limit - s.pos.Load()
		if remaining <= 0 {
			s.killLocked()
			return n, true, io.EOF
		}
		if n == len(p) {
			return n, true, nil
		}
		buf := p[n:]
		if int64(len(buf)) > remaining {
			buf = buf[:remaining]
		}

		read, readErr := s.body.Read(buf)
		n += read
		s.pos.Store(off + int64(n))
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			// A body is free to report its end alongside its last bytes rather
			// than on the read after them, and that is still a whole one.
			if s.pos.Load() >= s.limit {
				s.killLocked()
				return n, true, io.EOF
			}
			readErr = io.ErrUnexpectedEOF
		}
		s.killLocked()
		return n, true, readErr
	}
}

// openStream fires the lane's one request and puts what comes back through the
// gate a bounded read puts its own response through. That is also what
// verifies the reader's picture of the object where nothing has yet: when the
// stream is the first request the reader makes, this response is the lazy
// probe's.
//
// It is called with mu held, on a pending lane, and leaves that lane open or
// dead — never pending again. A view lying at or past the end of the object
// has nothing to stream and comes back as [io.EOF] on a dead lane.
func (r *ReaderAt) openStream() error {
	s := r.stream
	start := s.base

	req, err := r.newRequest(r.ctx)
	if err != nil {
		s.state = streamDead
		return err
	}
	// A view running to the end of the object, or past where the object is
	// already known to end, asks for the rest of it and lets the response say
	// where that is.
	wantEnd := int64(-1)
	if viewEnd := s.end(); viewEnd != math.MaxInt64 {
		if size, known := r.knownSize(); !known || viewEnd < size {
			wantEnd = viewEnd - 1
		}
	}
	if wantEnd < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, wantEnd))
	}
	ifRange := ifRangeValue(r.meta.Load())
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		s.state = streamDead
		return redactURLError(err)
	}

	granted, err := r.checkStream(resp, start, wantEnd, ifRange != "")
	if err != nil {
		drainAndClose(resp.Body)
		s.state = streamDead
		return err
	}

	s.body = resp.Body
	s.limit = granted
	s.pos.Store(start)
	s.state = streamOpen
	return nil
}

// checkStream decides whether the response opening the stream really is the
// object the reader was built against, from the offset the lane asked for,
// before any of its body is believed, and reports the absolute offset one past
// the last byte it grants. wantEnd is the last byte the request asked for, or
// -1 where it asked for everything from start on, and conditional says whether
// an If-Range rode along.
//
// What each status means here is what it means to a read; see
// [ReaderAt.ReadAt], whose taxonomy this one follows over a single response
// rather than one per read.
func (r *ReaderAt) checkStream(
	resp *http.Response,
	start, wantEnd int64,
	conditional bool,
) (int64, error) {
	if err := checkContentEncoding(resp); err != nil {
		return 0, fmt.Errorf(
			"httprange: streaming %s from offset %d: %w",
			redactRawURL(r.url), start, err,
		)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		return 0, r.streamUnsatisfied(resp, start)
	case http.StatusOK:
		// An empty entity is an object of zero bytes rather than a server
		// dropping the range, exactly as it is for a read; see
		// [ReaderAt.Probe].
		if resp.Header.Get("Content-Range") == "" && resp.ContentLength == 0 {
			if reason := r.pinOrVerify(resp, 0); reason != "" {
				return 0, fmt.Errorf(
					"%w: streaming %s from offset %d: %s",
					ErrObjectChanged, redactRawURL(r.url), start, reason,
				)
			}
			return 0, io.EOF
		}
		if conditional {
			return 0, fmt.Errorf(
				"%w: streaming %s from offset %d: If-Range did not match",
				ErrObjectChanged, redactRawURL(r.url), start,
			)
		}
		return 0, fmt.Errorf(
			"%w: streaming %s from offset %d",
			ErrRangeIgnored, redactRawURL(r.url), start,
		)
	default:
		return 0, &StatusCodeError{Code: resp.StatusCode}
	}

	first, last, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return 0, fmt.Errorf(
			"httprange: streaming %s from offset %d: %w",
			redactRawURL(r.url), start, err,
		)
	}
	// Ending short of what was asked for is the object's own end and nothing
	// else, the same rule a bounded read holds a 206 to. A short grant taken
	// as it stands would have the lane read the end of the body as the end of
	// the object and stop there, handing a truncated object back as a whole
	// one.
	if first != start || (wantEnd >= 0 && last > wantEnd) || (last != wantEnd && last != total-1) {
		return 0, fmt.Errorf(
			"%w: streaming %s from offset %d: got bytes %d-%d of %d",
			ErrObjectChanged, redactRawURL(r.url), start, first, last, total,
		)
	}
	if reason := r.pinOrVerify(resp, total); reason != "" {
		return 0, fmt.Errorf(
			"%w: streaming %s from offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), start, reason,
		)
	}
	return last + 1, nil
}

// streamUnsatisfied takes a 416 opening the stream for what it says, the way
// [ReaderAt.readUnsatisfied] does for a read: the view begins at or past the
// end of the object, which only a stream opened before the size was settled
// can ask for, and the complete length reported alongside the refusal settles
// it. There is then nothing to stream and the view is empty.
func (r *ReaderAt) streamUnsatisfied(resp *http.Response, start int64) error {
	first, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil || first != -1 {
		return &StatusCodeError{Code: resp.StatusCode}
	}
	if reason := r.pinOrVerify(resp, total); reason != "" {
		return fmt.Errorf(
			"%w: streaming %s from offset %d: %s",
			ErrObjectChanged, redactRawURL(r.url), start, reason,
		)
	}
	if start < total {
		return fmt.Errorf(
			"%w: streaming %s from offset %d: range refused within %d bytes",
			ErrObjectChanged, redactRawURL(r.url), start, total,
		)
	}
	return io.EOF
}
