package httprange_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/stream"
	"github.com/ngicks/go-fsys-helper/stream/httprange"
)

// The package must stay free of any dependency on the parent module, so the
// interfaces it is meant to fit are pinned from an external test file instead.
var (
	_ io.ReaderAt        = (*httprange.ReaderAt)(nil)
	_ io.Closer          = (*httprange.ReaderAt)(nil)
	_ stream.ReadAtSizer = (*httprange.ReaderAt)(nil)
)

func TestZipRoundTrip(t *testing.T) {
	archive, members := buildTestZip(t)
	srv := serveZip(t, archive)

	r, err := httprange.New(context.Background(), srv.URL+"/test.zip", nil)
	if err != nil {
		t.Fatalf("httprange.New: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("ReaderAt.Close: %v", err)
		}
	}()

	if r.Size() != int64(len(archive)) {
		t.Fatalf("ReaderAt.Size() = %d, want %d", r.Size(), len(archive))
	}

	sized := stream.SizedReadersFromReadAtSizer([]*httprange.ReaderAt{r})
	if len(sized) != 1 {
		t.Fatalf("SizedReadersFromReadAtSizer returned %d readers, want 1", len(sized))
	}
	if sized[0].Size != r.Size() {
		t.Errorf("SizedReaderAt.Size = %d, want %d", sized[0].Size, r.Size())
	}

	zr, err := zip.NewReader(r, r.Size())
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != len(members) {
		t.Fatalf("archive has %d members, want %d", len(zr.File), len(members))
	}

	for _, f := range zr.File {
		want, ok := members[f.Name]
		if !ok {
			t.Errorf("unexpected member %q", f.Name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Errorf("zip.File.Open(%q): %v", f.Name, err)
			continue
		}
		got, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Errorf("reading member %q: %v", f.Name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf(
				"member %q: got %d bytes differing from the %d written",
				f.Name, len(got), len(want),
			)
		}
	}
}

// buildTestZip returns an archive and the contents each of its members was
// built from.
func buildTestZip(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()

	names := []string{"small.txt", "large.txt"}
	members := map[string][]byte{
		"small.txt": []byte("hello from httprange\n"),
		"large.txt": zipPayload(300 << 10),
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, name := range names {
		// Stored rather than deflated: the point of the large member is that
		// reading it back takes many range requests, and compressing a
		// predictable pattern down to a few kilobytes would remove them.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatalf("zip.Writer.CreateHeader(%q): %v", name, err)
		}
		if _, err := w.Write(members[name]); err != nil {
			t.Fatalf("writing member %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Writer.Close: %v", err)
	}
	return buf.Bytes(), members
}

func zipPayload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// serveZip serves content over range requests, with the validators a reader
// pins itself to: the ETag is quoted so that it survives an If-Range, and the
// modification time is non-zero so that Last-Modified is sent at all.
func serveZip(t *testing.T, content []byte) *httptest.Server {
	t.Helper()

	modtime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("ETag", `"zip-test-v1"`)
			w.Header().Set("Content-Type", "application/zip")
			// A reader of its own per request: ServeContent seeks the one it
			// is handed, and requests run concurrently.
			http.ServeContent(w, req, "test.zip", modtime, bytes.NewReader(content))
		},
	))
	t.Cleanup(srv.Close)
	return srv
}
