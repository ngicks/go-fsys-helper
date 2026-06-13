# SUMMARY — F5 — Extract pure resume-trust decision from Pull

## What changed
`fsutil/resumable.go`: the correctness-critical "can the existing .part be
appended to?" logic — previously inlined twice inside Pull's ~180-line body
(the stage-A oversize/sidecar discard checks and the stage-B `trusted` boolean)
— is extracted into a single pure function:

```
func decideResume(partSize int64, storedETag string, expected, src ContentInfo) resumeDecision
```

`resumeDecision{startAt int64, discardPart bool}` says where to resume and
whether to throw the part away first. The rules (explicit, in one place):
1. `partSize <= 0` → start at 0, keep.
2. oversized (`expected.Size >= 0 && partSize > expected.Size`) → discard.
3. sidecar/expected ETag conflict (both known, different) → discard.
4. source-ETag untrusted (src ETag known, matches neither sidecar nor — when the
   sidecar is absent — expected) → discard.
5. otherwise resume at `partSize`.

## How Pull uses it (D8 preserved)
Pull keeps its two-phase open with no extra source round-trip:
- Pre-open: `decideResume(partSize, storedETag, expected, ContentInfo{Size: -1})`.
  The zero/unknown source makes the source-ETag trust check pass trivially, so
  only oversize and sidecar-conflict can discard — exactly the old stage-A
  guess. Pull opens the source at the resulting `startAt`.
- Post-open: `decideResume(partSize, storedETag, expected, srcInfo)`. Only when
  the guess was a resume (`startAt > 0`) and the now-known source identity does
  not back it up does Pull discard and reopen at 0 — the same single,
  proven-wrong-guess reopen the old code did.

No exported API changed; Pull's observable behavior is unchanged.

## Why
The decision was inlined three-way boolean logic in a long method; a wrong
`trusted == true` silently corrupts the output, and it was only testable by
driving an entire Pull. A pure function makes the whole trust matrix directly
unit-testable.

## Tests
Added `TestDecideResume` (table-driven) to `fsutil/resumable_test.go` covering
the full matrix: no-part base case, no-etag-world size resume, sidecar match,
expected-etag match (sidecar absent), source-vs-sidecar conflict,
source-vs-expected conflict, sidecar-vs-expected conflict, oversized part, the
two pre-open-guess cases (unknown source resumes / discards oversize), and the
unknown-expected-size (never-oversize) case. 11 cases, all asserting
`{startAt, discardPart}`.

## Test evidence
- `cd go-fsys-helper/fsutil && go build ./... && go vet ./... && go test -count=1 ./...` — PASS.
- `go test -run TestDecideResume -v ./` — all 11 matrix cases PASS.
- Existing resume / etag-mismatch integration tests in `resumable_test.go` and
  `resumable_fs_test.go` — PASS unchanged (behavior preserved).
- `cd go-fsys-helper/vroot && go build ./...` — PASS.
- `cd oci-image-copy && go build ./... && go test -count=1 ./pkg/imagecopy/...` — PASS.
