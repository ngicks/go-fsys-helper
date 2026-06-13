# SUMMARY — S4: Deduplicate close fan-out and response-body cleanup

## What changed

### `stream/multi_read_closer.go` + `stream/multi_read_at_closer.go`
- New unexported generic helper `closeFanOut[T](items []T, closeOne func(T) (err error, closed bool)) error`
  in multi_read_closer.go. It closes each item via `closeOne`, gathers errors
  prefixed `"index %d: "` (via serr.GatherPrefixed), and — crucially — skips an
  index entirely when `closeOne` reports `closed == false`.
- `multiReadCloser.Close` now calls `closeFanOut` with a closeOne that always
  closes (every element is an io.ReadCloser).
- `multiReadAtSeekCloser.Close` now calls `closeFanOut` with a closeOne that
  type-asserts `io.Closer` and returns `closed == false` for non-closer
  segments, preserving the original semantic difference between the two sites
  (ReadCloser elements close unconditionally; ReaderAt segments close only if
  they implement io.Closer). The now-unused `serr` import was dropped from
  multi_read_at_closer.go.

### `stream/fileserver/client.go`
- New unexported helper `drainClose(io.ReadCloser)` that does
  `io.Copy(io.Discard, body)` then `body.Close()` (errors ignored — the branch
  already returns a more meaningful error).
- Replaced the five repeated drain+close blocks with `drainClose`: the four
  non-success branches of `Get` (200-to-ranged-request, 206 Content-Range parse
  failure, 404/410, and default) and the `Put` post-response cleanup.
- `Stat` left unchanged: HEAD responses carry no body, and its existing bare
  `resp.Body.Close()` is documented as such; adding a drain there would be a
  pointless Copy and contradict that comment.

## Why
The close fan-out pattern existed in two diverged copies, and the
drain-and-close cleanup was copy-pasted five times across client.go error
branches — exactly the kind of duplication where a future edit lands in one copy
only. Both are now single-sourced.

## Tests added
- `stream/multi_read_at_closer_test.go`: `closeErrReaderAt` +
  `TestMultiReadAtSeekCloser_Close_errors` — a 3-segment reader where segments 0
  and 2 are closers that fail and segment 1 is a plain non-closer *bytes.Reader.
  Asserts the gathered error wraps both failures (errors.Is), carries the true
  indices `index 0:`/`index 2:`, and does NOT contain `index 1:` (non-closer
  skipped). This pins the index/conditional-close semantics of closeFanOut.
- `stream/fileserver/client_test.go`: `instrumentedBody` (records drained-to-EOF
  and closed) + `cannedResponse` + `TestHTTPClient_drains_non2xx` with subtests
  Get_404, Get_500_default, Get_200_ignored_range, and Put_500 — each asserts
  the body was drained to EOF and closed. Reuses the existing `doerFunc` adapter.

## Test evidence (commands + results)

    $ cd go-fsys-helper/stream && go build ./...   # exit=0
    $ go vet ./...                                  # exit=0
    $ go test ./...
    ok  github.com/ngicks/go-fsys-helper/stream
    ok  github.com/ngicks/go-fsys-helper/stream/fileserver                # exit=0

    $ go test -run 'TestMultiReadAtSeekCloser_Close_errors|TestHTTPClient_drains_non2xx' -v ./...
    --- PASS: TestMultiReadAtSeekCloser_Close_errors
    --- PASS: TestHTTPClient_drains_non2xx (Get_404, Get_500_default, Get_200_ignored_range, Put_500)
    PASS

    $ cd oci-image-copy && go test ./pkg/imagecopy/fileserver/...
    ok  github.com/ngicks/oci-image-copy/pkg/imagecopy/fileserver

## Deviations from plan
None. The 206 Content-Range-parse-failure branch is covered by drainClose but is
not separately instrumented in the test (it needs a malformed 206; the other
four branches plus Put give full coverage of the helper). Stat intentionally not
converted (no body).
