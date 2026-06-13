# SUMMARY — F2 — Surface destination Close() error in copyEntry

## What changed
`fsutil/copy_fs.go`, `copyEntry` regular-file branch: the destination file's
Close error is now surfaced on the success path instead of being discarded by a
bare `defer dstFile.Close()`.

- A `closed bool` guard plus a deferred `if !closed { _ = dstFile.Close() }`
  still closes the file on every early-error return (open-source, open-dest,
  copy error).
- After a successful `io.CopyBuffer`, the code sets `closed = true` and calls
  `dstFile.Close()` explicitly, returning its error.
- The source file close stays deferred (read side); this asymmetry intentionally
  matches `resumable_fs.go`'s write-side close handling.

## Why
On a network/buffer-backed Fsys the final flush happens during Close, so the
Close error is the only signal of a lost write. `copy_fs.go:216` previously
dropped it via `defer dstFile.Close()`, an internal inconsistency with
`resumable_fs.go` (which already surfaces its write-side close error) that has
data-integrity consequences for `CopyAll`, `CopyPath`, and
`SafeWriteOption.CopyFs`.

## Tests
Added mock to `fsutil/copy_fs_mock_test.go`:
- `closeErrFile` wraps an `*os.File`, closes the real fd, then returns a preset
  error from `Close` (simulating a flush-on-close failure).
- `closeErrFs` wraps `osfslite.OsfsLite` and returns `closeErrFile` values whose
  Close fails for a matching path.

Added test to `fsutil/copy_fs_error_test.go`:
- `TestCopyFs_ErrorPaths/CopyAll surfaces destination close error`: copies a
  regular file into a `closeErrFs` and asserts the injected close error surfaces
  from `CopyAll` via `errors.Is`.

## Test evidence
- `cd go-fsys-helper/fsutil && go build ./... && go vet ./... && go test ./...` — PASS.
- New subtest `CopyAll_surfaces_destination_close_error` — PASS; full
  `TestCopyFs_ErrorPaths` — PASS.
- `cd oci-image-copy && go build ./... && go test -count=1 ./pkg/imagecopy/...` —
  PASS (`pkg/imagecopy` and `pkg/imagecopy/fileserver` green against the F2
  fsutil change).

## Note
The oci-image-copy consumer build transiently failed earlier in this entry
because a concurrent agent had non-compiling work-in-progress in the sibling
`stream` module (which `pkg/imagecopy` imports via go.work). Once that module
settled the consumer build/test ran clean, as recorded above. No fsutil/F2
change was implicated.
