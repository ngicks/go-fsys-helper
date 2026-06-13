# SUMMARY — S1: Robust EOF handling and constructor input validation

## What changed

### `stream/multi_read_at_closer.go`
- `Read` and `readAt` now classify segment EOF with `errors.Is(err, io.EOF)`
  instead of bare `== io.EOF` (per DECISION.md D5). A segment that returns a
  *wrapped* EOF (`fmt.Errorf("...: %w", io.EOF)`) is now treated as a clean
  end-of-segment and the concatenation advances, instead of aborting with a
  spurious "read error" `MultiReadError`.
- After classification, when `isEOF` is true the returned `err` is normalized to
  the canonical `io.EOF`. This prevents a wrapped EOF surfaced from the *final*
  segment from leaking to callers (e.g. `io.ReadAll`, which compares against
  `io.EOF`).
- `NewMultiReadAtSeekCloser` panics if any `SizedReaderAt.Size` is negative
  (per DECISION.md D3). A negative size would silently corrupt the precomputed
  `headOff`/`upperLimit` math.

### `stream/seq_reader_at.go`
- `ReadAt`'s end-of-stream check now uses `errors.Is(err, io.EOF)` so a wrapped
  EOF from a user-supplied stream is handled; the returned error is normalized
  to canonical `io.EOF`.
- `NewSeqReaderAt` panics on a negative `size` (D3). Added `errors` to imports.

## Why it is safe
The existing per-segment size validation converts a read shorter than the
declared `SizedReaderAt.Size` into `io.ErrUnexpectedEOF` (the `rem > 0` branch).
Therefore widening EOF recognition from bare equality to `errors.Is` cannot mask
a short read / data loss — it strictly widens *correct* composition. This was
the supervisor-verified safety argument in the plan.

Bug-catching property confirmed in isolation: with the old `err != io.EOF` test a
wrapped EOF is classified as a hard error (`true`); with `!errors.Is(err, io.EOF)`
it is not (`false`). The new wrapped-EOF tests therefore fail against pre-fix code.

## Tests added
- `multi_read_closer_test.go`: `wrappedEofReaderAt` (returns wrapped `io.EOF` at
  segment end) and `prepareWrappedEofReader` helper.
- `multi_read_at_closer_test.go`:
  - `TestMultiReadAtSeekCloser_wrapped_eof` (subtests `Read`, `ReadAt`) — asserts
    the concatenation advances past every wrapped-EOF segment and yields the full
    `randomBytes` with no error / canonical `io.EOF`.
  - `TestMultiReadAtSeekCloser_negative_size` — asserts a negative `Size` panics
    with a message mentioning "negative Size".
- `seq_reader_at_test.go`:
  - `wrappedEofReadCloser` + `TestSeqReaderAt_wrapped_eof` — wrapped-EOF stream,
    clamped read returns available bytes and canonical `io.EOF`.
  - `TestSeqReaderAt_negative_size` — asserts negative size panics.

## Test evidence (commands + results)

    $ cd go-fsys-helper/stream && go build ./...      # exit=0
    $ go vet ./...                                     # exit=0
    $ go test ./...
    ok  github.com/ngicks/go-fsys-helper/stream
    ok  github.com/ngicks/go-fsys-helper/stream/fileserver
    ?   .../internal/serr        [no test files]
    ?   .../internal/testhelper  [no test files]                # exit=0

    $ go test -run 'TestMultiReadAtSeekCloser_wrapped_eof|TestMultiReadAtSeekCloser_negative_size|TestSeqReaderAt_wrapped_eof|TestSeqReaderAt_negative_size' -v ./...
    --- PASS: TestMultiReadAtSeekCloser_wrapped_eof (Read, ReadAt)
    --- PASS: TestMultiReadAtSeekCloser_negative_size
    --- PASS: TestSeqReaderAt_wrapped_eof
    --- PASS: TestSeqReaderAt_negative_size
    PASS

    $ cd oci-image-copy && go test ./pkg/imagecopy/fileserver/...
    ok  github.com/ngicks/oci-image-copy/pkg/imagecopy/fileserver

## Deviations from plan
None. Constructor validation implemented as panic per D3; EOF via `errors.Is`
per D5. One refinement beyond the literal plan text: the returned error is
*normalized* to canonical `io.EOF` after an `errors.Is` match, which is required
for the "yield full data" guarantee when the wrapped EOF originates from the
final segment.
