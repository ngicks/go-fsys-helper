package httprange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// readInChunks reads rd to its end n bytes at a time, which is the scan a
// caller declaring a section up front makes.
func readInChunks(t *testing.T, rd io.Reader, n int) []byte {
	t.Helper()

	var (
		got []byte
		buf = make([]byte, n)
	)
	for {
		read, err := rd.Read(buf)
		got = append(got, buf[:read]...)
		if errors.Is(err, io.EOF) {
			return got
		}
		if err != nil {
			t.Fatalf("reading returned error: %v", err)
		}
	}
}

// TestNewRange_noRequest states that declaring a section is as free as
// declaring a whole object: the stream is a request the first read makes.
func TestNewRange_noRequest(t *testing.T) {
	content := testContent(256)

	for _, tc := range []struct {
		name   string
		cfg    *Config
		base   int64
		length int64
	}{
		{name: "nothing_known", base: 0, length: math.MaxInt64},
		{name: "size_known", cfg: &Config{Size: int64(len(content))}, base: 64, length: 128},
		{name: "empty_section", base: 0, length: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startConformantServer(t, content)

			r, err := NewRange(context.Background(), s.URL, tc.base, tc.length, tc.cfg)
			if err != nil {
				t.Fatalf("NewRange returned error: %v", err)
			}
			defer r.Close()

			if got := s.requestCount(); got != 0 {
				t.Fatalf("NewRange made %d requests, want 0", got)
			}
		})
	}
}

func TestNewRange_rejects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rawURL  string
		off     int64
		wantMsg string
	}{
		{
			name:   "negative_offset",
			rawURL: "http://example.test/obj", off: -1,
			wantMsg: "negative offset",
		},
		{
			// Whatever New refuses to build over, NewRange refuses too.
			name:   "url_of_the_wrong_kind",
			rawURL: "ftp://example.test/obj", off: 0,
			wantMsg: "is not http or https",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := doerFunc(func(req *http.Request) (*http.Response, error) {
				t.Errorf("NewRange issued a request to %q", req.URL.Redacted())
				return nil, errors.New("no request was expected")
			})

			r, err := NewRange(
				context.Background(), tc.rawURL, tc.off, 16, &Config{Client: client},
			)
			if err == nil {
				r.Close()
				t.Fatal("NewRange returned no error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("NewRange returned %v, want it to say %q", err, tc.wantMsg)
			}
		})
	}
}

// TestNewRange_wholeObject is the full copy: the caller says up front that they
// will read the object front to back, and the whole walk costs the one request
// the stream is.
func TestNewRange_wholeObject(t *testing.T) {
	content := testContent(4096)
	s := startConformantServer(t, content)

	r, err := NewRange(context.Background(), s.URL, 0, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	got := readInChunks(t, io.NewSectionReader(r, 0, math.MaxInt64), 64)
	if !bytes.Equal(got, content) {
		t.Fatalf("read %d bytes differing from the %d served", len(got), len(content))
	}
	if n := s.requestCount(); n != 1 {
		t.Fatalf("copying the object cost %d requests, want 1", n)
	}
}

// TestNewRange_resume is the download picked up where an earlier one stopped:
// the section runs from what is already saved to the end of the object, and
// the validators saved with those bytes ride the one request it takes.
func TestNewRange_resume(t *testing.T) {
	const (
		savedETag = `"v1"`
		saved     = 256
	)
	content := testContent(1024)
	s := startConformantServer(t, content)

	r, err := NewRange(
		context.Background(), s.URL, saved, math.MaxInt64, &Config{ETag: savedETag},
	)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	got := readInChunks(t, io.NewSectionReader(r, 0, math.MaxInt64), 128)
	if !bytes.Equal(got, content[saved:]) {
		t.Fatalf("read %d bytes, want the %d left after %d", len(got), len(content)-saved, saved)
	}

	headers := s.requestHeaders()
	if len(headers) != 1 {
		t.Fatalf("resuming cost %d requests, want 1", len(headers))
	}
	if got := headers[0].Get("Range"); got != "bytes=256-" {
		t.Fatalf("the stream asked for Range %q, want %q", got, "bytes=256-")
	}
	if got := headers[0].Get("If-Range"); got != savedETag {
		t.Fatalf("the stream sent If-Range %q, want %q", got, savedETag)
	}
}

// TestNewRange_boundedSection covers the section that ends before the object
// does: what it hands out is its own bytes and nothing past them, however much
// of the object is left.
func TestNewRange_boundedSection(t *testing.T) {
	content := testContent(1024)
	size := int64(len(content))

	t.Run("read_to_its_end", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			base   int64
			length int64
			want   []byte
		}{
			{name: "inside_the_object", base: 128, length: 256, want: content[128:384]},
			// A section longer than what is left ends where the object does.
			{name: "past_the_object_end", base: 896, length: 256, want: content[896:]},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := startConformantServer(t, content)

				r, err := NewRange(context.Background(), s.URL, tc.base, tc.length, nil)
				if err != nil {
					t.Fatalf("NewRange returned error: %v", err)
				}
				defer r.Close()

				got := readInChunks(t, io.NewSectionReader(r, 0, math.MaxInt64), 64)
				if !bytes.Equal(got, tc.want) {
					t.Fatalf(
						"read %d bytes differing from the %d in the section",
						len(got),
						len(tc.want),
					)
				}
				if n := s.requestCount(); n != 1 {
					t.Fatalf("reading the section cost %d requests, want 1", n)
				}
			})
		}
	})

	t.Run("offsets_are_relative", func(t *testing.T) {
		const (
			base   = 128
			length = 256
		)
		s := startConformantServer(t, content)

		r, err := NewRange(context.Background(), s.URL, base, length, nil)
		if err != nil {
			t.Fatalf("NewRange returned error: %v", err)
		}
		defer r.Close()

		buf := make([]byte, 16)
		if n, err := r.ReadAt(buf, 0); n != len(buf) || err != nil {
			t.Fatalf("ReadAt = (%d, %v), want (%d, <nil>)", n, err, len(buf))
		}
		if !bytes.Equal(buf, content[base:base+int64(len(buf))]) {
			t.Fatalf("ReadAt at offset 0 returned %x, want the section's first bytes", buf)
		}
	})

	t.Run("boundary_eof", func(t *testing.T) {
		const (
			base   = 128
			length = 256
		)
		// The read straddling the end of the section is made after the stream
		// is gone, so it is the bounded path that has to stop there: the object
		// runs on for another 640 bytes.
		for _, tc := range []struct {
			name  string
			off   int64
			wantN int
		}{
			{name: "straddling_it", off: length - 8, wantN: 8},
			{name: "reaching_it_exactly", off: length - 16, wantN: 16},
			{name: "at_it", off: length},
			{name: "past_it", off: length * 2},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := startConformantServer(t, content)

				r, err := NewRange(context.Background(), s.URL, base, length, nil)
				if err != nil {
					t.Fatalf("NewRange returned error: %v", err)
				}
				defer r.Close()

				buf := make([]byte, 16)
				n, err := r.ReadAt(buf, tc.off)
				if n != tc.wantN || !errors.Is(err, io.EOF) {
					t.Fatalf(
						"ReadAt(make([]byte, %d), %d) = (%d, %v), want (%d, %v)",
						len(buf), tc.off, n, err, tc.wantN, io.EOF,
					)
				}
				want := content[base+tc.off : base+tc.off+int64(n)]
				if n > 0 && !bytes.Equal(buf[:n], want) {
					t.Fatalf("ReadAt returned %x, want %x", buf[:n], want)
				}
			})
		}
	})

	// A section starting where the object ends is a section with nothing in it,
	// which the object's own size settles rather than the section's length.
	t.Run("at_the_object_end", func(t *testing.T) {
		s := startConformantServer(t, content)

		r, err := NewRange(
			context.Background(), s.URL, size, math.MaxInt64, &Config{Size: size},
		)
		if err != nil {
			t.Fatalf("NewRange returned error: %v", err)
		}
		defer r.Close()

		if n, err := r.ReadAt(make([]byte, 16), 0); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
		}
		if got := s.requestCount(); got != 0 {
			t.Fatalf("the read cost %d requests, want none past a known end", got)
		}
	})
}

// TestNewRange_emptySection states what a section with nothing in it does: the
// same thing an [io.SectionReader] built that way does, at no cost.
func TestNewRange_emptySection(t *testing.T) {
	content := testContent(256)

	for _, length := range []int64{0, -1} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			s := startConformantServer(t, content)

			r, err := NewRange(context.Background(), s.URL, 0, length, nil)
			if err != nil {
				t.Fatalf("NewRange returned error: %v", err)
			}
			defer r.Close()

			if n, err := r.ReadAt(make([]byte, 16), 0); n != 0 || !errors.Is(err, io.EOF) {
				t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
			}
			if got := s.requestCount(); got != 0 {
				t.Fatalf("reading an empty section cost %d requests, want 0", got)
			}
		})
	}
}

// TestNewRange_pastUnknownEnd covers the section nobody could know was empty:
// the stream opens, the server refuses the range and reports the complete
// length, and that settles both the size and the section.
func TestNewRange_pastUnknownEnd(t *testing.T) {
	content := testContent(256)
	size := int64(len(content))
	s := startConformantServer(t, content)

	r, err := NewRange(context.Background(), s.URL, size*2, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	if n, err := r.ReadAt(make([]byte, 16), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("the read cost %d requests, want the stream's own", got)
	}
	if got, ok := r.Metadata(); got.Size != size || !ok {
		t.Fatalf("Metadata() = (%+v, %t), want size %d, settled", got, ok, size)
	}
	// The refusal settled the size, so the read after it asks nothing.
	if n, err := r.ReadAt(make([]byte, 16), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt = (%d, %v), want (0, %v)", n, err, io.EOF)
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("the second read cost %d requests in all, want the stream's own", got)
	}
}

// TestNewRange_outOfOrderRead states what a read arriving anywhere but where
// the stream stands costs: the stream, for good.
func TestNewRange_outOfOrderRead(t *testing.T) {
	content := testContent(1024)
	s := startConformantServer(t, content)

	r, err := NewRange(context.Background(), s.URL, 0, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	read := func(off int64) {
		t.Helper()

		buf := make([]byte, 64)
		n, err := r.ReadAt(buf, off)
		if n != len(buf) || err != nil {
			t.Fatalf(
				"ReadAt(make([]byte, %d), %d) = (%d, %v), want (%d, <nil>)",
				len(buf),
				off,
				n,
				err,
				len(buf),
			)
		}
		if !bytes.Equal(buf, content[off:off+int64(n)]) {
			t.Fatalf("ReadAt(make([]byte, %d), %d) read the wrong bytes", len(buf), off)
		}
	}

	read(0)
	read(64)
	if got := s.requestCount(); got != 1 {
		t.Fatalf("the reads in order cost %d requests, want the stream's own", got)
	}

	read(512)
	assertLaneDead(t, r)
	if got := s.requestCount(); got != 2 {
		t.Fatalf("the read out of order cost %d requests in all, want 2", got)
	}

	// The stream is never re-armed, so reads that look in order from here are
	// bounded requests like any other.
	read(128)
	read(192)
	if got := s.requestCount(); got != 4 {
		t.Fatalf("the reads after it cost %d requests in all, want one each", got)
	}
}

// TestNewRange_streamDiesMidBody covers the connection that fails halfway: the
// read hands back what it got along with the error, and nothing reopens the
// stream — the reads after it are bounded requests, and whole.
func TestNewRange_streamDiesMidBody(t *testing.T) {
	content := testContent(256)
	s := startHandlerServer(t, handleSequence(
		// A response promising the whole object and handing over a fraction of
		// it, which is what the client sees of a connection dying.
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set(
				"Content-Range",
				fmt.Sprintf("bytes 0-%d/%d", len(content)-1, len(content)),
			)
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[:32])
		},
		handlePartial(content, `"v1"`),
	))

	r, err := NewRange(context.Background(), s.URL, 0, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 64)
	n, err := r.ReadAt(buf, 0)
	if n != 32 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadAt = (%d, %v), want (32, %v)", n, err, io.ErrUnexpectedEOF)
	}
	if !bytes.Equal(buf[:n], content[:n]) {
		t.Fatalf("ReadAt returned %x, want %x", buf[:n], content[:n])
	}
	assertLaneDead(t, r)

	if n, err := r.ReadAt(buf, 32); n != len(buf) || err != nil {
		t.Fatalf("ReadAt after the stream died = (%d, %v), want (%d, <nil>)", n, err, len(buf))
	}
	if !bytes.Equal(buf, content[32:32+len(buf)]) {
		t.Fatalf("ReadAt after the stream died read the wrong bytes")
	}
}

// TestNewRange_concurrentReads runs the reader the way its documentation says
// it may be run: one caller walking the object front to back while others read
// wherever they like. Every read is answered correctly, whichever of them the
// stream is serving and whichever of them ends it.
func TestNewRange_concurrentReads(t *testing.T) {
	content := testContent(8192)
	size := int64(len(content))
	s := startConformantServer(t, content)

	r, err := NewRange(context.Background(), s.URL, 0, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}
	defer r.Close()

	var wg sync.WaitGroup
	wg.Go(func() {
		var (
			got []byte
			buf = make([]byte, 256)
		)
		for {
			off := int64(len(got))
			n, err := r.ReadAt(buf, off)
			got = append(got, buf[:n]...)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("the walking ReadAt at offset %d returned error: %v", off, err)
				return
			}
		}
		if !bytes.Equal(got, content) {
			t.Errorf("the walk read %d bytes differing from the %d served", len(got), len(content))
		}
	})
	for i := range 8 {
		wg.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(i), uint64(size)))
			for range 16 {
				off := rng.Int64N(size)
				buf := make([]byte, 1+rng.IntN(512))
				n, err := r.ReadAt(buf, off)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Errorf("ReadAt(make([]byte, %d), %d) returned error: %v", len(buf), off, err)
					return
				}
				if !bytes.Equal(buf[:n], content[off:off+int64(n)]) {
					t.Errorf("ReadAt(make([]byte, %d), %d) read the wrong bytes", len(buf), off)
					return
				}
			}
		})
	}
	wg.Wait()
}

// TestNewRange_CloseDuringStreamRead states that closing never waits for the
// network: the stream's body is what a blocked read is holding, and cancelling
// is what pries it loose.
func TestNewRange_CloseDuringStreamRead(t *testing.T) {
	content := testContent(256)
	arrived := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	// The response promises the whole object, hands over a few bytes and stops
	// there, leaving the read that wanted more waiting on the body.
	s := startHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Content-Range",
			fmt.Sprintf("bytes 0-%d/%d", len(content)-1, len(content)),
		)
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[:16])
		w.(http.Flusher).Flush()

		close(arrived)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})

	r, err := NewRange(context.Background(), s.URL, 0, math.MaxInt64, nil)
	if err != nil {
		t.Fatalf("NewRange returned error: %v", err)
	}

	read := make(chan error, 1)
	go func() {
		_, err := r.ReadAt(make([]byte, 128), 0)
		read <- err
	}()

	<-arrived
	closed := make(chan error, 1)
	go func() { closed <- r.Close() }()

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return while a stream read was blocked on the body")
	}
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("the blocked ReadAt returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the blocked ReadAt did not return after Close")
	}
}
