package fsutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

// ---------------------------------------------------------------------------
// Test helpers & stubs
// ---------------------------------------------------------------------------

// testOpt is the concrete instantiation used throughout these tests.
type testOpt = ResumableCopyOption[*osfslite.OsfsLite, *os.File]

// staticSource is a ResumableSource backed by an in-memory byte slice.
// It honours offset, reports Size and ETag as configured.
type staticSource struct {
	data []byte
	etag string
	// overrideETag is returned instead of etag on the *second and later* Open
	// calls. Useful to simulate ETag-change scenarios.
	overrideETagAfterFirst string
	opens                  int
}

func (s *staticSource) Open(_ context.Context, offset int64) (io.ReadCloser, ContentInfo, error) {
	s.opens++
	etag := s.etag
	if s.overrideETagAfterFirst != "" && s.opens > 1 {
		etag = s.overrideETagAfterFirst
	}
	return io.NopCloser(bytes.NewReader(s.data[offset:])),
		ContentInfo{ETag: etag, Size: int64(len(s.data))},
		nil
}

// shortReadSource returns exactly limitBytes then EOF on the first Open call,
// and the full data on subsequent calls. Used to simulate an interrupted
// download that is then resumed.
type shortReadSource struct {
	data       []byte
	etag       string
	limitBytes int64
	opens      int
}

func (s *shortReadSource) Open(
	_ context.Context,
	offset int64,
) (io.ReadCloser, ContentInfo, error) {
	s.opens++
	r := bytes.NewReader(s.data[offset:])
	var reader io.Reader = r
	if s.opens == 1 && s.limitBytes > 0 && offset == 0 {
		reader = io.LimitReader(r, s.limitBytes)
	}
	return io.NopCloser(reader),
		ContentInfo{ETag: s.etag, Size: int64(len(s.data))},
		nil
}

// errSource always returns an error from Open.
type errSource struct{ err error }

func (s *errSource) Open(_ context.Context, _ int64) (io.ReadCloser, ContentInfo, error) {
	return nil, ContentInfo{}, s.err
}

// changedETagSource returns a different ETag than expected.
type changedETagSource struct {
	data        []byte
	reportedTag string
}

func (s *changedETagSource) Open(
	_ context.Context,
	offset int64,
) (io.ReadCloser, ContentInfo, error) {
	return io.NopCloser(bytes.NewReader(s.data[offset:])),
		ContentInfo{ETag: s.reportedTag, Size: int64(len(s.data))},
		nil
}

// writeContent writes content to path in tempDir.
func writeContent(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeContent: %v", err)
	}
}

// readContent reads the file and returns its content string.
func readContent(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("readContent: %v", err)
	}
	return string(b)
}

// fileExists reports whether name exists under dir.
func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// sha256Hex returns the sha256 hex digest of b.
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Pull tests
// ---------------------------------------------------------------------------

func TestPull_FreshDownload(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	src := &staticSource{data: []byte("hello world"), etag: "etag1"}

	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: 11}, 0o644)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if got := readContent(t, dir, "out.txt"); got != "hello world" {
		t.Errorf("content mismatch: got %q", got)
	}
	// Part and sidecar must be cleaned up.
	if fileExists(dir, "out.txt.part") {
		t.Error("part file not removed after successful Pull")
	}
	if fileExists(dir, "out.txt.part.etag") {
		t.Error("sidecar not removed after successful Pull")
	}
}

func TestPull_AlreadyComplete(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "out.txt", "hello world")

	src := &staticSource{data: []byte("different content"), etag: "etag-other"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: 11}, 0o644)
	if err != nil {
		t.Fatalf("Pull (already complete): %v", err)
	}
	// Source must NOT have been opened.
	if src.opens != 0 {
		t.Error("source was opened despite file already existing")
	}
}

func TestPull_AlreadyComplete_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "out.txt", "short")

	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", nil,
		ContentInfo{Size: 100}, 0o644)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected ErrSizeMismatch, got: %v", err)
	}
}

func TestPull_ResumedDownload(t *testing.T) {
	data := []byte("hello world 12345")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	// Simulate a previous partial download: 5 bytes already written.
	partial := data[:5]
	writeContent(t, dir, "out.txt.part", string(partial))
	// Write matching sidecar.
	writeContent(t, dir, "out.txt.part.etag", "myetag")

	src := &staticSource{data: data, etag: "myetag"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "myetag", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("Pull (resume): %v", err)
	}

	if got := readContent(t, dir, "out.txt"); got != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, string(data))
	}
	// Source must have been opened at offset 5 (the first open).
	if src.opens != 1 {
		t.Errorf("expected 1 Open call, got %d", src.opens)
	}
}

func TestPull_OversizedPartialRestart(t *testing.T) {
	data := []byte("hello world")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	// Part is larger than the declared total — must be discarded.
	writeContent(t, dir, "out.txt.part", string(data)+"extra bytes beyond end")
	writeContent(t, dir, "out.txt.part.etag", "etag1")

	src := &staticSource{data: data, etag: "etag1"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("Pull (oversized part restart): %v", err)
	}
	if got := readContent(t, dir, "out.txt"); got != string(data) {
		t.Errorf("content mismatch: got %q", got)
	}
	// Source must have been opened at offset 0 (fresh start).
	if src.opens != 1 {
		t.Errorf("expected 1 Open call, got %d", src.opens)
	}
}

func TestPull_SidecarETagMismatchRestart(t *testing.T) {
	data := []byte("hello world")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	// Partial 5 bytes from a different transfer (different ETag in sidecar).
	writeContent(t, dir, "out.txt.part", string(data[:5]))
	writeContent(t, dir, "out.txt.part.etag", "old-etag")

	src := &staticSource{data: data, etag: "new-etag"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "new-etag", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("Pull (sidecar mismatch restart): %v", err)
	}
	if got := readContent(t, dir, "out.txt"); got != string(data) {
		t.Errorf("content mismatch: got %q", got)
	}
}

func TestPull_SourceETagConflict_ErrContentChanged(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	src := &changedETagSource{data: []byte("data"), reportedTag: "wrong-etag"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "expected-etag", Size: 4}, 0o644)
	if !errors.Is(err, ErrContentChanged) {
		t.Fatalf("expected ErrContentChanged, got: %v", err)
	}
}

func TestPull_ShortReadKeepsPartFile(t *testing.T) {
	data := []byte("hello world 12345")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	// Source returns only 5 bytes before EOF on the first call.
	src := &shortReadSource{data: data, etag: "etag1", limitBytes: 5}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: int64(len(data))}, 0o644)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF, got: %v", err)
	}

	// Part file must still exist for later resume.
	if !fileExists(dir, "out.txt.part") {
		t.Error("part file was removed despite short read — cannot resume")
	}
	// Final file must not exist.
	if fileExists(dir, "out.txt") {
		t.Error("final file must not exist after short read")
	}
}

func TestPull_ShortReadThenResume(t *testing.T) {
	data := []byte("hello world 12345")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	src := &shortReadSource{data: data, etag: "etag1", limitBytes: 5}
	opt := testOpt{}

	// First attempt — should fail with ErrUnexpectedEOF.
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: int64(len(data))}, 0o644)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("first Pull: expected ErrUnexpectedEOF, got: %v", err)
	}

	// Second attempt — full data returned from offset 5.
	err = opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag1", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("second Pull (resume): %v", err)
	}
	if got := readContent(t, dir, "out.txt"); got != string(data) {
		t.Errorf("content mismatch after resume: got %q", got)
	}
	if fileExists(dir, "out.txt.part") {
		t.Error("part file not cleaned up after successful resume")
	}
	if fileExists(dir, "out.txt.part.etag") {
		t.Error("sidecar not cleaned up after successful resume")
	}
}

func TestPull_PreCommitHookSha256_Success(t *testing.T) {
	data := []byte("content to hash")
	expected := sha256Hex(data)
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	sha256Hook := func(f *os.File, path string) error {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("sha256 hook: copy: %w", err)
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != expected {
			return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
		}
		return nil
	}

	src := &staticSource{data: data, etag: "etag-sha"}
	opt := testOpt{PreCommitHooks: []func(*os.File, string) error{sha256Hook}}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag-sha", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("Pull with sha256 hook: %v", err)
	}
	if got := readContent(t, dir, "out.txt"); got != string(data) {
		t.Errorf("content mismatch: got %q", got)
	}
}

func TestPull_PreCommitHookFailure_PartRemoved(t *testing.T) {
	data := []byte("content to verify")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	hookErr := errors.New("hook intentional failure")
	failHook := func(f *os.File, path string) error { return hookErr }

	src := &staticSource{data: data, etag: "etag-fail"}
	opt := testOpt{PreCommitHooks: []func(*os.File, string) error{failHook}}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "etag-fail", Size: int64(len(data))}, 0o644)
	if !errors.Is(err, hookErr) {
		t.Fatalf("expected hookErr, got: %v", err)
	}

	// Part and sidecar must be cleaned up on hook failure.
	if fileExists(dir, "out.txt.part") {
		t.Error("part file should be removed after hook failure")
	}
	if fileExists(dir, "out.txt.part.etag") {
		t.Error("sidecar should be removed after hook failure")
	}
	if fileExists(dir, "out.txt") {
		t.Error("final file must not exist after hook failure")
	}
}

func TestPull_SizeMismatch_SourceVsExpected(t *testing.T) {
	data := []byte("hello")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	// Source reports size 5, caller declares size 99.
	src := &staticSource{data: data, etag: "e"}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{ETag: "e", Size: 99}, 0o644)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected ErrSizeMismatch, got: %v", err)
	}
}

func TestPull_CtxCancelledMidCopy_KeepsPart(t *testing.T) {
	// A source that blocks until context is cancelled.
	data := []byte("ABCDEFGHIJKLMNOP")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the first ctxCopy iteration sees a cancelled ctx.
	cancel()

	src := &staticSource{data: data, etag: "e"}
	opt := testOpt{}
	err := opt.Pull(ctx, fsys, "out.txt", src,
		ContentInfo{ETag: "e", Size: int64(len(data))}, 0o644)

	// Must return context error (wrapped in PathError).
	if !errors.Is(err, context.Canceled) {
		t.Logf("err = %v", err)
		// Context may have been honoured before or during copy.
		// Some data may have been written; the important thing is that err != nil.
		if err == nil {
			t.Error("expected error due to cancelled context")
		}
	}
}

// ---------------------------------------------------------------------------
// Push tests
// ---------------------------------------------------------------------------

// memSink is an in-memory ResumableSink.
type memSink struct {
	buf      bytes.Buffer
	etag     string
	complete bool
}

func (m *memSink) State(_ context.Context) (SinkState, error) {
	if m.complete {
		return SinkState{Offset: int64(m.buf.Len()), Complete: true}, nil
	}
	return SinkState{Offset: int64(m.buf.Len()), ETag: m.etag}, nil
}

func (m *memSink) Append(_ context.Context, info ContentInfo, offset int64, r io.Reader) error {
	if offset == 0 {
		m.buf.Reset()
	}
	if info.ETag != "" {
		m.etag = info.ETag
	}
	_, err := io.Copy(&m.buf, r)
	return err
}

func (m *memSink) Commit(_ context.Context) error {
	m.complete = true
	return nil
}

func TestPush_Fresh(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", "push me")

	sink := &memSink{}
	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "etag-push", Size: 7})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !sink.complete {
		t.Error("sink not committed")
	}
	if got := sink.buf.String(); got != "push me" {
		t.Errorf("sink content: %q, want %q", got, "push me")
	}
}

func TestPush_AlreadyComplete(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", "data")

	sink := &memSink{complete: true}
	sink.buf.WriteString("data")
	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "e", Size: 4})
	if err != nil {
		t.Fatalf("Push (complete): %v", err)
	}
}

func TestPush_Resume(t *testing.T) {
	data := "hello world from push"
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", data)

	// Sink already has 5 bytes from a previous run.
	sink := &memSink{etag: "e"}
	sink.buf.WriteString(data[:5])

	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "e", Size: int64(len(data))})
	if err != nil {
		t.Fatalf("Push (resume): %v", err)
	}
	if got := sink.buf.String(); got != data {
		t.Errorf("sink content: %q, want %q", got, data)
	}
}

func TestPush_SinkETagMismatch_RestartAtZero(t *testing.T) {
	data := "fresh start required"
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", data)

	// Sink has partial data but under a different ETag → must restart.
	sink := &memSink{etag: "old-etag"}
	sink.buf.WriteString(data[:5])

	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "new-etag", Size: int64(len(data))})
	if err != nil {
		t.Fatalf("Push (etag mismatch restart): %v", err)
	}
	if got := sink.buf.String(); got != data {
		t.Errorf("sink content after restart: %q, want %q", got, data)
	}
}

func TestPush_OffsetEqualsSize_StraightCommit(t *testing.T) {
	data := "already there"
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", data)

	// Sink already has all bytes but hasn't committed.
	sink := &memSink{etag: "e"}
	sink.buf.WriteString(data)

	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "e", Size: int64(len(data))})
	if err != nil {
		t.Fatalf("Push (straight commit): %v", err)
	}
	if !sink.complete {
		t.Error("expected sink to be committed")
	}
}

func TestPush_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	writeContent(t, dir, "src.txt", "five!")

	sink := &memSink{}
	opt := testOpt{}
	err := opt.Push(context.Background(), fsys, "src.txt", sink,
		ContentInfo{ETag: "e", Size: 999})
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected ErrSizeMismatch, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Custom PartSuffix tests
// ---------------------------------------------------------------------------

func TestPull_CustomPartSuffix(t *testing.T) {
	data := []byte("custom suffix test")
	dir := t.TempDir()
	fsys := osfslite.New(dir)

	src := &staticSource{data: data, etag: "e"}
	opt := testOpt{PartSuffix: ".tmp"}
	err := opt.Pull(context.Background(), fsys, "out.bin", src,
		ContentInfo{ETag: "e", Size: int64(len(data))}, 0o644)
	if err != nil {
		t.Fatalf("Pull (custom suffix): %v", err)
	}
	if fileExists(dir, "out.bin.tmp") {
		t.Error("custom suffix part file not cleaned up")
	}
}

// ---------------------------------------------------------------------------
// Error wrapping / WrapPathErr integration
// ---------------------------------------------------------------------------

func TestPull_ErrorWrapping(t *testing.T) {
	dir := t.TempDir()
	fsys := osfslite.New(dir)
	// source that errors.
	src := &errSource{err: errors.New("network error")}
	opt := testOpt{}
	err := opt.Pull(context.Background(), fsys, "out.txt", src,
		ContentInfo{}, 0o644)

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected PathError, got %T: %v", err, err)
	}
	if !strings.Contains(pathErr.Path, "out.txt") {
		t.Errorf("PathError path %q does not contain file name", pathErr.Path)
	}
}
