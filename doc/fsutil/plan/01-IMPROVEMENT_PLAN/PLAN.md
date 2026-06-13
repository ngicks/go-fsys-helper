# fsutil — Improvement Plan 01

Scope: module `github.com/ngicks/go-fsys-helper/fsutil`.
Origin: design review (multi-agent, supervisor-verified against source on 2026-06-13).
Baseline: all tests green before work started.

In-workspace consumers: `vroot` (heavy: `WrapPathErr` 40+ sites in synthfs,
`ResolveSymlink` in walk), `oci-image-copy/pkg/imagecopy` (`SafeWriteOption`,
resumable Pull/Push, `FsSink`/`FsSource`).

## Entry F1 — WrapPathErr/WrapLinkErr must not mutate the caller's error

Evidence (verified):
- `wrap_path_err.go:18-26`: when `err` is already `*fs.PathError`, the function
  rewrites `Op`/`Path` on the SAME struct and returns it. Same for `*os.LinkError`
  at `:41-52`. Any error value shared (sentinel-ish, stored, or observed by
  another goroutine) is silently rewritten under the caller; the helper is also
  non-idempotent across calls.

Change: copy-on-write — allocate a fresh `fs.PathError`/`os.LinkError` copy,
merge non-zero fields into the copy, return the copy. Behavior-compatible for all
observed call sites (they use only the return value).

Verification: regression test asserting the input struct is unchanged after the
call; fsutil + vroot test suites green.
Size: S.

## Entry F2 — Surface destination Close() error in copyEntry

Evidence (verified):
- `copy_fs.go:216`: `defer dstFile.Close()` discards the close error; on
  network/buffer-backed Fsys the final flush error is the only signal of a lost
  write. The module itself guards this elsewhere (`resumable_fs.go` write-side
  close handling), so this is an internal inconsistency with data-integrity
  consequences for `CopyAll`/`CopyPath`/`SafeWriteOption.CopyFs`.

Change: explicit close on the success path; return its error. Source-file close
stays deferred (read side, asymmetry matches resumable_fs).

Verification: new mock-File test asserting a destination close error surfaces
from `CopyAll`; fsutil + oci-image-copy tests green.
Size: S.

## Entry F3 — Deduplicate part/sidecar machinery between resumable.go and resumable_fs.go

Evidence (verified):
- `resumable.go:167,184,204` (generic `removePartFiles`/`readSidecar`/`writeSidecar`)
  vs `resumable_fs.go:131-172` (`FsSink.sfx/partPath/sidecarPath/readSidecar/writeSidecar`)
  implement the same algorithms twice, including independently hard-coded `0o644`
  sidecar perms. Pull/Push interoperability depends on the two sides staying
  semantically symmetric; duplication is where they will drift.

Change: extract one unexported shared helper set (over the existing generic
constraints) owning part path naming, sidecar read/write, and part-file removal;
both `Pull` and `FsSink` use it. No exported signature changes.

Verification: full `resumable_test.go` + `resumable_fs_test.go` suites green
unchanged.
Size: M.

## Entry F4 — Single-source hooks for SafeWriteOption (drop positional hook params)

Evidence (verified):
- `safe_write.go:51` struct fields `PreHooks/PostHooks` AND positional
  `preHooks, postHooks` params on `Write`/`Copy`/`CopyFs` (`:63,94,130,164`); the
  execution order across the two sources is non-obvious. The only production
  caller (`oci-image-copy/pkg/imagecopy/fsocidirs.go:129-136`) passes `nil, nil`.

Change: remove the positional params; hooks live only on the struct (matches
`ResumableCopyOption.PreCommitHooks` single-source design). Per-call hooks =
per-call option value. Update the one production call site and the tests that
exercised arg-hooks.

Verification: fsutil + oci-image-copy build and tests green.
Size: M (mostly test updates).

## Entry F5 — Extract pure resume-trust decision from Pull

Evidence (verified):
- `resumable.go:341` (`trusted := srcInfo.ETag == "" || ...`) plus the stage-A
  discard checks around `:308-313`: the correctness-critical "can the existing
  .part be appended to?" decision is inlined three-way boolean logic in a
  ~180-line method; wrong `trusted==true` means silent corruption; currently only
  testable by driving the whole Pull.

Change: extract a pure `decideResume(partSize, storedETag, expected, src) →
{startAt, discardPart}` consulted by Pull. Preserve the existing two-phase
open-at-guessed-offset / reopen-at-0 sequencing (no extra source round-trip).

Verification: new table-driven unit test covering the trust matrix (no-etag
world, sidecar match, expected-etag match, conflict, oversized part); existing
resume/etag-mismatch integration tests green.
Size: M.

## Entry F6 — Minor consistency: fs.ModeSymlink in resolve_symlink.go

Evidence (verified for the cosmetic part):
- `resolve_symlink.go` mixes `os.ModeSymlink` and `fs.ModeSymlink` for the same
  bit; `os` is imported solely for that.

Change: use `fs.ModeSymlink` consistently; drop the `os` import.
NOT taken: unexporting `ResolvePath`/`SyncHook`/`ChmodMask` (see DECISION.md D5).
Size: XS (folded into the F1 commit if convenient, else its own).
