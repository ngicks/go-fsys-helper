# SUMMARY — S2: Fix wrong MultiReadError.Index; unify segment-read; complete Error()

## What changed (`stream/multi_read_at_closer.go`)

1. **New shared helper `readSegment(idx int, p []byte, off int64)`** — the single
   place that performs one segment ReadAt + EOF classification + size validation,
   always recording the *absolute* segment index `idx` in any `*MultiReadError`.
   It returns `(n, isEOF, err)`; the `isEOF` flag lets the stateful `Read` decide
   whether to advance its cursor without re-running the classification.

2. **`Read` index bug fixed.** `Read` searches the tail `r.r[r.idx:]`, getting a
   *relative* index `i`; it now computes the absolute index `idx = r.idx + i` and
   passes it to `readSegment`. Previously, on a hard error with `n == 0`, the
   guard `if n > 0 || err == io.EOF` skipped the `r.idx += i` advance, so the
   `MultiReadError` was built with the stale base `r.idx` instead of the failing
   segment's absolute index. Reachable whenever a zero-length segment makes
   `search` skip forward (`i > 0`). The cursor still advances only on
   `n > 0 || isEOF` (unchanged behavior), but the error index is now correct
   independently of the advance.

3. **`readAt` deduplicated.** It now reduces to: bounds check → `search` →
   `readSegment`. The previously-duplicated read/validate/wrap block is gone, so
   the two paths can no longer diverge.

4. **`Error()` completed.** It now prints `totalOff` (virtual offset) and
   `bufLen` in addition to the existing fields — the most useful coordinates for
   locating a fault in a concatenated stream.

## Why
The Read and ReadAt paths held two copies of the same read+validate+wrap logic
that had already diverged (ReadAt indexed absolutely and correctly; Read used a
relative base and reported a stale index). Collapsing them into one helper fixes
the Read-path bug and prevents future drift.

Bug-catching property confirmed in isolation: with the old Read logic the
reported index is the stale base `0`; the new code reports the absolute `1`.
The new test asserts `Index == 1`, so it fails against pre-fix code.

## Tests added (`stream/multi_read_at_closer_test.go`)
- `hardErrReaderAt` — an io.ReaderAt that always returns `(0, err)`.
- `TestMultiReadAtSeekCloser_error_index_after_zero_length` (subtests `Read`,
  `ReadAt`) — segments `[zero-length, failing]`; asserts exact
  `Index == 1`, `ReaderOff == 0`, `TotalOff == 0`, `BufLen == 8`, and
  `errors.Is(err, sentinel)`. The Read subtest fails before the fix.
- `TestMultiReadError_Error_includes_totalOff_bufLen` — asserts `Error()` now
  contains `totalOff = …` and `bufLen = …`.

## Test evidence (commands + results)

    $ cd go-fsys-helper/stream && go build ./...   # exit=0
    $ go vet ./...                                  # exit=0
    $ go test ./...
    ok  github.com/ngicks/go-fsys-helper/stream
    ok  github.com/ngicks/go-fsys-helper/stream/fileserver
    ?   .../internal/serr        [no test files]
    ?   .../internal/testhelper  [no test files]               # exit=0

    $ go test -run 'TestMultiReadAtSeekCloser_error_index_after_zero_length|TestMultiReadError_Error_includes_totalOff_bufLen' -v .
    --- PASS: TestMultiReadAtSeekCloser_error_index_after_zero_length (Read, ReadAt)
        logged: "MultiReadError: idx = 1, off = 0, totalOff = 0, bufLen = 8, err = boom, cause = read error"
    --- PASS: TestMultiReadError_Error_includes_totalOff_bufLen
    PASS

    # fuzz corpora (existing seeds + exploration), 5s each, all PASS:
    $ go test -fuzz 'FuzzMultiReadAtSeekCloser_Read$'   -fuzztime 5s .   # PASS
    $ go test -fuzz 'FuzzMultiReadAtSeekCloser_ReadAt$' -fuzztime 5s .   # PASS
    $ go test -fuzz 'FuzzMultiReadAtSeekCloser_Seek$'   -fuzztime 5s .   # PASS

    $ cd oci-image-copy && go test ./pkg/imagecopy/fileserver/...
    ok  github.com/ngicks/oci-image-copy/pkg/imagecopy/fileserver

## Deviations from plan
None. The exported `MultiReadError` struct shape is unchanged; only its `Error()`
rendering gained two fields, and the helper is unexported.
