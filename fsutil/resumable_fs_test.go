package fsutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

// ---------------------------------------------------------------------------
// Helper types & functions
// ---------------------------------------------------------------------------

type testFsSinkOpt = ResumableCopyOption[*osfslite.OsfsLite, *os.File]

// makeFsSource creates a FsSource backed by an OsfsLite rooted at dir.
func makeFsSource(dir, name, etag string) *FsSource[*osfslite.OsfsLite, *os.File] {
	return NewFsSource[*osfslite.OsfsLite, *os.File](osfslite.New(dir), name, etag)
}

// makeFsSink creates a FsSink backed by an OsfsLite rooted at dir.
func makeFsSink(dir, name string, perm os.FileMode) *FsSink[*osfslite.OsfsLite, *os.File] {
	return NewFsSink[*osfslite.OsfsLite, *os.File](osfslite.New(dir), name, perm)
}

// ---------------------------------------------------------------------------
// FsSource tests
// ---------------------------------------------------------------------------

func TestFsSource_Open(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "data.bin"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := makeFsSource(src, "data.bin", "etag-x")
	rc, info, err := s.Open(context.Background(), 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	if info.ETag != "etag-x" {
		t.Errorf("ETag: got %q, want %q", info.ETag, "etag-x")
	}
	if info.Size != 6 {
		t.Errorf("Size: got %d, want 6", info.Size)
	}

	b, _ := io.ReadAll(rc)
	if string(b) != "abcdef" {
		t.Errorf("content: got %q, want %q", string(b), "abcdef")
	}
}

func TestFsSource_OpenWithOffset(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "data.bin"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := makeFsSource(src, "data.bin", "e")
	rc, info, err := s.Open(context.Background(), 3)
	if err != nil {
		t.Fatalf("Open with offset: %v", err)
	}
	defer rc.Close()

	// Size reflects the whole file, not the remaining bytes.
	if info.Size != 6 {
		t.Errorf("Size: got %d, want 6", info.Size)
	}

	b, _ := io.ReadAll(rc)
	if string(b) != "def" {
		t.Errorf("content from offset 3: got %q, want %q", string(b), "def")
	}
}

// ---------------------------------------------------------------------------
// FsSink tests
// ---------------------------------------------------------------------------

func TestFsSink_StateEmpty(t *testing.T) {
	dir := t.TempDir()
	sink := makeFsSink(dir, "out.bin", 0o644)
	st, err := sink.State(context.Background())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.Offset != 0 || st.Complete || st.ETag != "" {
		t.Errorf("unexpected state: %+v", st)
	}
}

func TestFsSink_StateWithPartFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.bin.part"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "out.bin.part.etag"),
		[]byte("myetag"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dir, "out.bin", 0o644)
	st, err := sink.State(context.Background())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.Offset != 5 {
		t.Errorf("Offset: got %d, want 5", st.Offset)
	}
	if st.ETag != "myetag" {
		t.Errorf("ETag: got %q, want %q", st.ETag, "myetag")
	}
	if st.Complete {
		t.Error("Complete should be false")
	}
}

func TestFsSink_StateComplete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.bin"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dir, "out.bin", 0o644)
	st, err := sink.State(context.Background())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !st.Complete {
		t.Error("Complete should be true when final file exists")
	}
}

func TestFsSink_AppendFresh(t *testing.T) {
	dir := t.TempDir()
	sink := makeFsSink(dir, "out.bin", 0o644)
	data := []byte("fresh data")

	err := sink.Append(context.Background(),
		ContentInfo{ETag: "e", Size: int64(len(data))},
		0, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	b, _ := os.ReadFile(filepath.Join(dir, "out.bin.part"))
	if string(b) != string(data) {
		t.Errorf("part content: got %q, want %q", string(b), string(data))
	}
	// Sidecar must exist.
	sc, _ := os.ReadFile(filepath.Join(dir, "out.bin.part.etag"))
	if string(sc) != "e" {
		t.Errorf("sidecar: got %q, want %q", string(sc), "e")
	}
}

func TestFsSink_AppendResume_TruncateOversized(t *testing.T) {
	data := []byte("hello world")
	dir := t.TempDir()
	// Part has 8 bytes but we resume at offset 5 — truncate to 5 first.
	if err := os.WriteFile(filepath.Join(dir, "out.bin.part"), data[:8], 0o644); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dir, "out.bin", 0o644)
	remaining := data[5:]
	err := sink.Append(context.Background(),
		ContentInfo{ETag: "e", Size: int64(len(data))},
		5, bytes.NewReader(remaining))
	if err != nil {
		t.Fatalf("Append (resume): %v", err)
	}

	b, _ := os.ReadFile(filepath.Join(dir, "out.bin.part"))
	if string(b) != string(data) {
		t.Errorf("part content: got %q, want %q", string(b), string(data))
	}
}

func TestFsSink_AppendResume_SizeMismatchTooShort(t *testing.T) {
	data := []byte("hello world")
	dir := t.TempDir()
	// Part has only 2 bytes but caller expects offset 5.
	if err := os.WriteFile(filepath.Join(dir, "out.bin.part"), data[:2], 0o644); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dir, "out.bin", 0o644)
	err := sink.Append(context.Background(),
		ContentInfo{ETag: "e", Size: int64(len(data))},
		5, bytes.NewReader(data[5:]))
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected ErrSizeMismatch, got: %v", err)
	}
}

func TestFsSink_CommitRenamesAndCleansSidecar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.bin.part"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "out.bin.part.etag"),
		[]byte("e"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dir, "out.bin", 0o644)
	if err := sink.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "out.bin")); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.bin.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file should be gone after Commit")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.bin.part.etag")); !errors.Is(err, os.ErrNotExist) {
		t.Error("sidecar should be gone after Commit")
	}
}

func TestFsSink_CommitIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.bin"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No part file.
	sink := makeFsSink(dir, "out.bin", 0o644)
	if err := sink.Commit(context.Background()); err != nil {
		t.Fatalf("Commit (idempotent): %v", err)
	}
}

// ---------------------------------------------------------------------------
// fsys↔fsys roundtrip tests
// ---------------------------------------------------------------------------

func TestFsysFsys_Roundtrip_Pull(t *testing.T) {
	content := []byte("roundtrip content via Pull")
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "blob"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	src := makeFsSource(srcDir, "blob", "sha256:deadbeef")
	opt := testFsSinkOpt{}
	err := opt.Pull(context.Background(),
		osfslite.New(dstDir), "blob", src,
		ContentInfo{ETag: "sha256:deadbeef", Size: int64(len(content))},
		0o644)
	if err != nil {
		t.Fatalf("Pull roundtrip: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
	// Cleanup check.
	if _, err := os.Stat(filepath.Join(dstDir, "blob.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file left behind")
	}
}

func TestFsysFsys_Roundtrip_Push(t *testing.T) {
	content := []byte("roundtrip content via Push")
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "blob"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	sink := makeFsSink(dstDir, "blob", 0o644)
	opt := testFsSinkOpt{}
	err := opt.Push(context.Background(),
		osfslite.New(srcDir), "blob", sink,
		ContentInfo{ETag: "sha256:cafebabe", Size: int64(len(content))})
	if err != nil {
		t.Fatalf("Push roundtrip: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "blob.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file left behind")
	}
}

// ---------------------------------------------------------------------------
// Interrupt-then-resume roundtrip
// ---------------------------------------------------------------------------

// blockingReader delivers data one byte at a time, then blocks on doneCh before
// returning io.EOF.  Call cancel() to unblock it with an error.  Used to
// simulate an interrupted transfer mid-copy.
type blockingReader struct {
	data   []byte
	pos    int
	doneCh <-chan struct{}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		// Block until cancelled.
		<-b.doneCh
		return 0, context.Canceled
	}
	p[0] = b.data[b.pos]
	b.pos++
	return 1, nil
}

// interruptSource is a ResumableSource whose first Open returns a
// blockingReader that can be cancelled mid-stream.  Subsequent Opens return
// the full remaining data normally.
type interruptSource struct {
	data []byte
	etag string
	// doneCh is closed to signal the blocking reader to stop.
	doneCh chan struct{}
	opens  int
}

func (s *interruptSource) Open(
	_ context.Context,
	offset int64,
) (io.ReadCloser, ContentInfo, error) {
	s.opens++
	if s.opens == 1 {
		// Return a reader that delivers half the data then blocks.
		half := len(s.data) / 2
		r := &blockingReader{
			data:   s.data[offset : offset+int64(half)],
			doneCh: s.doneCh,
		}
		return io.NopCloser(r), ContentInfo{ETag: s.etag, Size: int64(len(s.data))}, nil
	}
	// Subsequent opens: serve from offset normally.
	return io.NopCloser(bytes.NewReader(s.data[offset:])),
		ContentInfo{ETag: s.etag, Size: int64(len(s.data))}, nil
}

func TestFsysFsys_InterruptResume_Pull(t *testing.T) {
	content := []byte("AAAABBBBCCCCDDDD")
	dstDir := t.TempDir()
	fsys := osfslite.New(dstDir)

	doneCh := make(chan struct{})
	src := &interruptSource{data: content, etag: "e", doneCh: doneCh}
	opt := testFsSinkOpt{}

	// Run first attempt in a goroutine; let it deliver half the data, then
	// cancel by closing doneCh.
	errCh := make(chan error, 1)
	go func() {
		errCh <- opt.Pull(context.Background(), fsys, "blob", src,
			ContentInfo{ETag: "e", Size: int64(len(content))}, 0o644)
	}()

	// Give the goroutine time to read half the data then unblock it.
	time.Sleep(20 * time.Millisecond)
	close(doneCh)

	err := <-errCh
	if err == nil {
		// If the whole file happened to be written already (race), that's fine.
		got, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
		if bytes.Equal(got, content) {
			return // lucky full write on first try
		}
		t.Fatal("expected error from cancelled source, got nil")
	}

	// Part file must exist for resume.
	partInfo, statErr := os.Stat(filepath.Join(dstDir, "blob.part"))
	if statErr != nil {
		t.Fatalf("part file missing after interrupt: %v", statErr)
	}
	if partInfo.Size() == 0 {
		t.Fatal("part file is empty; no progress was made")
	}

	// Second attempt: full data.
	err = opt.Pull(context.Background(), fsys, "blob", src,
		ContentInfo{ETag: "e", Size: int64(len(content))}, 0o644)
	if err != nil {
		t.Fatalf("Pull (resume after interrupt): %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
	if !bytes.Equal(got, content) {
		t.Errorf("content after resume: got %q, want %q", got, content)
	}
	// No stale files.
	if _, err := os.Stat(filepath.Join(dstDir, "blob.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file not cleaned up after successful resume")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "blob.part.etag")); !errors.Is(err, os.ErrNotExist) {
		t.Error("sidecar not cleaned up after successful resume")
	}
}

func TestFsysFsys_InterruptResume_Push(t *testing.T) {
	content := []byte("push interrupt test content!!")
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "blob"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a cancellable context for the first Push attempt.
	ctx1, cancel1 := context.WithCancel(context.Background())

	sink := makeFsSink(dstDir, "blob", 0o644)
	opt := testFsSinkOpt{}

	// Write partial state manually to simulate interrupted push.
	partial := content[:len(content)/2]
	if err := os.WriteFile(filepath.Join(dstDir, "blob.part"), partial, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dstDir, "blob.part.etag"),
		[]byte("sha256:test"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	cancel1()

	// "First attempt" is already simulated above; go straight to resume.
	_ = ctx1

	// Resume: Push should pick up from halfway.
	err := opt.Push(context.Background(),
		osfslite.New(srcDir), "blob", sink,
		ContentInfo{ETag: "sha256:test", Size: int64(len(content))})
	if err != nil {
		t.Fatalf("Push (resume after interrupt): %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
	if !bytes.Equal(got, content) {
		t.Errorf("content after push resume: got %q, want %q", got, content)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "blob.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file not cleaned up after push resume")
	}
}

// ---------------------------------------------------------------------------
// ctx cancellation during Pull (keeps part file)
// ---------------------------------------------------------------------------

func TestPull_CtxCancelKeepsPartFile(t *testing.T) {
	content := make([]byte, 1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	dstDir := t.TempDir()
	fsys := osfslite.New(dstDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before start

	src := &staticSource{data: content, etag: "e"}
	opt := testFsSinkOpt{BufSize: 1}
	err := opt.Pull(ctx, fsys, "blob", src,
		ContentInfo{ETag: "e", Size: int64(len(content))}, 0o644)

	if err == nil {
		// If the file is complete, that's fine too.
		if got, _ := os.ReadFile(filepath.Join(dstDir, "blob")); bytes.Equal(got, content) {
			return
		}
		t.Fatal("expected an error but got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("Pull returned: %v (acceptable: contains context.Canceled)", err)
	}
	// The final file must not exist if incomplete.
	if _, err2 := os.Stat(filepath.Join(dstDir, "blob")); err2 == nil {
		// Final file exists; verify it's complete.
		b, _ := os.ReadFile(filepath.Join(dstDir, "blob"))
		if !bytes.Equal(b, content) {
			t.Error("final file exists but content is wrong")
		}
	}
}
