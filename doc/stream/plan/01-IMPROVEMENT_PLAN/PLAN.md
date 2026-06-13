# stream — Improvement Plan 01

Scope: module `github.com/ngicks/go-fsys-helper/stream`.
Origin: design review (multi-agent, supervisor-verified against source on 2026-06-13).
Baseline: all tests green before work started.

In-workspace consumers (blast radius reference):
- `tarfs/reader.go`: `SizedReaderAt`, `NewMultiReadAtSeekCloser`, `NewByteRepeater`
- `oci-image-copy/pkg/imagecopy/fileserver/adapter.go`: `fileserver.NewChunkedSource`, `fileserver.NewChunkedSink`
- `NewSeqReaderAt` only used internally by `ChunkedSource.Open` (`fileserver/chunk.go`)

## Entry S1 — Robust EOF handling and constructor input validation

Evidence (verified):
- `multi_read_at_closer.go:148,157,164,166,242,249,251` compare with bare `== io.EOF`
  while reading from arbitrary user-supplied `io.ReaderAt` segments. A segment
  returning a wrapped EOF (`fmt.Errorf("...: %w", io.EOF)`) is treated as a hard
  error, aborting the concatenation instead of advancing.
- `NewMultiReadAtSeekCloser` (`:120`) and `NewSeqReaderAt` accept negative sizes,
  silently corrupting `headOff`/`upperLimit` math.

Change:
- Use `errors.Is(err, io.EOF)` in `multi_read_at_closer.go` and `seq_reader_at.go`
  EOF checks. Safe: the existing `rem` validation converts short reads into
  `io.ErrUnexpectedEOF`, so wrapped EOF cannot mask data loss.
- Constructor validation for negative size (mechanism per DECISION.md D3).

Verification: new test with a segment returning wrapped EOF (must advance and
yield full data); negative-size constructor test; all existing tests + fuzz green.
Size: S.

## Entry S2 — Fix wrong MultiReadError.Index in Read path; unify duplicated segment-read logic; complete Error() output

Evidence (verified):
- `multi_read_at_closer.go:141-158`: `Read` computes `i` relative to `r.r[r.idx:]`;
  on a hard error with `n == 0` the guard at `:148` skips `r.idx += i`, then the
  `MultiReadError` built at `:154` reports the stale base `r.idx` instead of the
  failing segment's absolute index `r.idx + i`. Reachable when zero-length
  segments cause `search` to skip forward (`i > 0`).
- `readAt` (`:224-255`) duplicates the same read+validate+wrap logic with correct
  absolute indexing — two copies have already diverged.
- `Error()` (`:40-45`) omits `TotalOff` and `BufLen`, the most useful fields for
  locating a fault in a concatenated stream.

Change:
- Extract one shared per-segment helper used by both `Read` and `readAt`, taking
  the absolute segment index; always build `MultiReadError` with absolute index.
- Include `TotalOff`/`BufLen` in `Error()`.

Verification: new table test asserting exact `Index`/`ReaderOff`/`TotalOff` for a
failing segment positioned after a zero-length segment (fails before the fix in
the Read path); existing wrong-size tests + fuzz green.
Size: M.

## Entry S3 — Make seqReaderAt honor the io.ReaderAt contract; harden reopen error path

Evidence (verified):
- `seq_reader_at.go:33-40` documents the type as NOT safe for concurrent use even
  though its only purpose is to be an `io.ReaderAt`, whose contract allows
  parallel `ReadAt`. Composing it into `NewMultiReadAtSeekCloser` (as
  `fileserver/chunk.go` does) silently makes the composite's advertised
  concurrency-safe `ReadAt` racy.
- `seq_reader_at.go:90-92`: stale-stream `Close()` failure is returned bare, with
  no context, inconsistent with every other error in the file.
- After failures that nil `r.current`, `r.off` keeps a stale value; the
  "off is valid iff current != nil" invariant is implicit only.

Change:
- Guard `ReadAt` state with a `sync.Mutex` (per DECISION.md D2) and update docs of
  both `NewSeqReaderAt` and `NewMultiReadAtSeekCloser`.
- Wrap the stale-stream close error with context; reset `r.off = 0` whenever
  `r.current` is set to nil.
- NOT taken: drain-forward optimization (see DECISION.md D4).

Verification: new `-race` test firing concurrent `ReadAt`s at disjoint offsets
(races before the fix); existing seq tests green.
Size: S.

## Entry S4 — Deduplicate close fan-out and response-body cleanup

Evidence (verified):
- `multi_read_at_closer.go:257-265` and `multi_read_closer.go:33-39` repeat the
  same "close each, gather prefixed errors" pattern.
- `fileserver/client.go:181,197,206,213,281`: `io.Copy(io.Discard, resp.Body)` +
  `Close` cleanup repeated five times across error branches.

Change:
- Shared unexported helper for prefixed close fan-out (each caller passes its own
  typed closer slice, preserving the existing semantic difference: ReadCloser
  elements close unconditionally; ReaderAt segments close only if io.Closer).
- `drainClose(io.ReadCloser)` helper in `fileserver`, used by every non-success
  branch of `Get`/`Stat`/`Put`.

Verification: existing Close-error tests and `fileserver/client_test.go` green;
add an instrumented-body test asserting non-2xx responses are drained to EOF and
closed.
Size: S.

## Explicitly not planned
- `cancellable.go`, `ByteRepeater`, `SizedReadersFrom*`: reviewed, no change
  warranted (documented design intent, no consumers harmed).
