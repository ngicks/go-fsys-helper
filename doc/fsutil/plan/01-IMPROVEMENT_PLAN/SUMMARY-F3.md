# SUMMARY — F3 — Deduplicate part/sidecar machinery

## What changed
The .part-file / ETag-sidecar machinery was implemented twice — once as generic
helpers in `resumable.go` (used by `Pull`) and once as `FsSink` methods in
`resumable_fs.go` — including independently hard-coded `0o644` sidecar perms and
duplicate `.part` / `.part.etag` naming. F3 collapses this into one unexported
shared helper set.

`fsutil/resumable.go` now owns:
- `const defaultPartSuffix = ".part"` and `const sidecarPerm fs.FileMode = 0o644`
  (single source of both previously-duplicated literals).
- `partPaths(name, suffix) (partPath, sidecarPath)` — the one place that derives
  `name+suffix` and `+".etag"`.
- `partSidecarFile` / `partSidecarFsys[File]` — deliberately narrow constraints
  (Read/Write/Close on the file; OpenFile/Remove on the fsys) so any caller,
  including `FsSink`'s wider `fsSinkFsys`/`fsSinkFile`, can reuse the helpers.
- `removePartFiles` / `readSidecar` / `writeSidecar` re-generalised over those
  narrow constraints; `writeSidecar` now uses `sidecarPerm`.

`fsutil/resumable.go` `Pull` uses `partPaths(name, opt.partSuffix())` and
`opt.partSuffix()` returns `defaultPartSuffix`.

`fsutil/resumable_fs.go` `FsSink`:
- `sfx()` returns `defaultPartSuffix`.
- `partPath()` / `sidecarPath()` delegate to `partPaths`.
- `readSidecar()` / `writeSidecar()` delegate to the shared generic helpers
  (`readSidecar[Fsys, File]` / `writeSidecar[Fsys, File]`); the ~30 lines of
  duplicated OpenFile/ReadAll/WriteString/Close logic and the second `0o644`
  literal are deleted. The now-unused `strings` import is removed.

No exported signature changed.

## Why
`Pull`/`FsSink` interoperability depends on the two sides keeping their .part +
sidecar handling semantically symmetric; the duplication (two ETag-perm
literals, two naming derivations, two read/write copies) is exactly where they
would drift. One shared helper set makes drift impossible.

## Tests
No new tests (pure refactor; D-level behavior unchanged). Verification is the
existing `resumable_test.go` + `resumable_fs_test.go` suites passing unchanged.

## Test evidence
- `cd go-fsys-helper/fsutil && go build ./... && go vet ./... && go test -count=1 ./...` — PASS.
- `go test -run 'Resumable|FsSink|FsSource|Pull|Push' ./...` — PASS.
- `cd go-fsys-helper/vroot && go build ./...` — PASS (repeatable), confirming the
  vroot consumer compiles against the refactored fsutil (no exported API change).

## Note on vroot test suite
`go test ./...` in vroot produced *nondeterministic* TestRoot/Escapes failures
across memfs/osfs/synthfs during this entry. Root cause: a concurrent agent is
mid-rewrite of `vroot/acceptancetest/escapes.go` (uncommitted, ~231 lines:
adds SetupExternal + PathError-path-leak assertions); the saved-but-incomplete
harness flickers between compiling/failing/passing moment to moment, uniformly
across all three backends. The failures are in the vroot test harness, not in
fsutil: vroot builds cleanly against this fsutil on every attempt, and F3 makes
no exported API or behavior change. (The same harness rewrite was green during
the F1 entry earlier in this session.)
