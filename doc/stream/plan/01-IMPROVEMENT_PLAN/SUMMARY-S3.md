# SUMMARY — S3: seqReaderAt honors io.ReaderAt contract; reopen path hardened

## What changed (`stream/seq_reader_at.go`)

1. **Concurrency: internal mutex (DECISION.md D2).** Added `mu sync.Mutex` to
   `seqReaderAt`; `ReadAt` and `Close` now lock it. This makes the type honor the
   [io.ReaderAt] contract (which permits concurrent ReadAt) rather than carrying
   a prose-only "NOT safe for concurrent use" warning that composition silently
   violated (ChunkedSource -> MultiReadAtSeekCloser). The lock is uncontended on
   the sequential hot path. The clamp (offset/size bounds, p slicing) runs before
   the lock since it only reads immutable fields.

2. **Stale-stream close error wrapped.** When a reopen must first close the
   current stream and that Close fails, the error is now wrapped with context and
   the offset:
   `"NewSeqReaderAt ReadAt: closing stale stream before reopen at offset %d: %w"`,
   consistent with the file's other errors (previously returned bare).

3. **Invariant made explicit: off reset to 0 with current.** Every place that
   sets `r.current = nil` (Close, reopen close failure, reopen close success, EOF,
   non-EOF error) now also sets `r.off = 0`, so a stale offset can never be
   mistaken for a live stream position. Documented on the struct field.

4. **Docs updated** on both `NewSeqReaderAt` ("Safe for concurrent use…", noting
   concurrent disjoint reads trade open count for safety) and
   `NewMultiReadAtSeekCloser` (its ReadAt is concurrency-safe only when every
   segment's ReadAt is; NewSeqReaderAt and io.SectionReader segments qualify).

5. **NOT taken: drain-forward optimization (DECISION.md D4).** Forward jumps
   still reopen; no behavior change there.

## Tests added (`stream/seq_reader_at_test.go`)
- `TestSeqReaderAt_concurrent_ReadAt` — 32 goroutines each sweep all 256-byte
  chunks of a 32 KiB buffer starting at interleaved offsets, asserting correct
  bytes. Run with `-race` it flags a data race before the mutex and passes after.
- `TestSeqReaderAt_stale_close_error` (with `errCloseReadCloser`) — a reopen
  whose stale-stream Close fails returns an error that wraps the close error and
  mentions "stale stream" and the offset; after the failure the reader recovers
  (off reset) and a subsequent read succeeds.

## Test evidence (commands + results)

    $ cd go-fsys-helper/stream && go build ./...   # exit=0
    $ go vet ./...                                  # exit=0
    $ go test ./...
    ok  github.com/ngicks/go-fsys-helper/stream
    ok  github.com/ngicks/go-fsys-helper/stream/fileserver                 # exit=0
    $ go test -race ./...
    ok  github.com/ngicks/go-fsys-helper/stream            (1.059s)
    ok  github.com/ngicks/go-fsys-helper/stream/fileserver  (1.168s)      # exit=0

    $ go test -race -run 'TestSeqReaderAt' -v .
    --- PASS: (all 11 TestSeqReaderAt_* including concurrent_ReadAt, stale_close_error)
    PASS

    # Race-catching property verified by temporarily removing the ReadAt lock:
    #   go test -race -run TestSeqReaderAt_concurrent_ReadAt .  -> "WARNING: DATA RACE"
    # Restored -> ok.

    $ cd oci-image-copy && go test ./pkg/imagecopy/fileserver/...
    ok  github.com/ngicks/oci-image-copy/pkg/imagecopy/fileserver

## Deviations from plan
None. D2 mutex, D4 (no drain-forward), and the reopen-path hardening implemented
as specified. The clamp runs before acquiring the lock (it touches only
immutable size); this is a placement detail, not a semantic deviation.
