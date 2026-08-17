package httprange_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/httprange"
)

// TestFullCopy_oneRequest is what the streaming reader is for, stated end to
// end against a server this package had no hand in: a multi-megabyte object
// copied off net/http's own file server, byte for byte, leaving a single line
// in its access log.
func TestFullCopy_oneRequest(t *testing.T) {
	const size = 4 << 20

	content := randomPayload(size)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "object.bin"), content, 0o600); err != nil {
		t.Fatalf("writing the object to serve: %v", err)
	}

	var requests atomic.Int64
	srv := httptest.NewServer(countRequests(&requests, http.FileServer(http.Dir(dir))))
	t.Cleanup(srv.Close)

	r, err := httprange.NewRange(
		context.Background(), srv.URL+"/object.bin", 0, math.MaxInt64, nil,
	)
	if err != nil {
		t.Fatalf("httprange.NewRange: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("ReaderAt.Close: %v", err)
		}
	}()

	var got bytes.Buffer
	n, err := io.Copy(&got, io.NewSectionReader(r, 0, math.MaxInt64))
	if err != nil {
		t.Fatalf("copying the object: %v", err)
	}
	if n != size || !bytes.Equal(got.Bytes(), content) {
		t.Fatalf("copied %d bytes differing from the %d served", n, size)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("copying the object cost %d requests, want 1", got)
	}
}

// countRequests counts what reaches h, which is the access log the copy above
// is measured against.
func countRequests(n *atomic.Int64, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h.ServeHTTP(w, r)
	})
}

// randomPayload returns n bytes nothing repeats over, so that bytes landing at
// an offset other than their own cannot pass for the ones that belong there.
func randomPayload(n int) []byte {
	b := make([]byte, n)
	rng := rand.New(rand.NewPCG(0x9e3779b9, 0x7f4a7c15))
	var word [8]byte
	for i := 0; i < len(b); i += len(word) {
		binary.LittleEndian.PutUint64(word[:], rng.Uint64())
		copy(b[i:], word[:])
	}
	return b
}
