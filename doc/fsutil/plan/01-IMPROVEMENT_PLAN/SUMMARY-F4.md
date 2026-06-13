# SUMMARY — F4 — Single-source hooks for SafeWriteOption (breaking)

## What changed (breaking API change — the only one in plan 01)
`fsutil/safe_write.go`: the positional `preHooks, postHooks []func(f File,
path string) error` parameters were removed from `Write`, `Copy`, and `CopyFs`,
and from the internal `safeOperation`. Hooks now live ONLY on the
`SafeWriteOption` struct fields `PreHooks`/`PostHooks`.

New signatures:
- `Write(fsys Fsys, name string, writeFunc func(io.Writer) error, perm fs.FileMode) error`
- `Copy(fsys Fsys, name string, r io.Reader, perm fs.FileMode) error`
- `CopyFs(fsys Fsys, name string, src fs.FS, perm fs.FileMode) error`

`safeOperation` now runs `opt.PreHooks` → operation → `opt.PostHooks` (the
previous interleaving of struct hooks with argument hooks is gone). The
`PreHooks`/`PostHooks` struct doc comment was updated to state they are the
single source (matching `ResumableCopyOption.PreCommitHooks`) and that per-call
hooks are expressed via a per-call option value; the invariant typos were fixed.

## Why (decision D6)
The struct already carried `PreHooks`/`PostHooks` AND every method took
positional `preHooks, postHooks` too; the execution order across the two
sources was non-obvious (struct-pre → arg-pre → op → arg-post → struct-post).
The only production caller passed `nil, nil`, all consumers are in-workspace,
and `ResumableCopyOption` already uses a single struct-only hook source.
Collapsing to struct-only removes the confusing dual ordering.

## Call-site updates
- Production caller `oci-image-copy/pkg/imagecopy/fsocidirs.go`: dropped the two
  trailing `nil, nil` args from `safeWriteOpt.Copy(...)`. (Committed separately
  in the oci-image-copy repo on branch main, message
  `imagecopy: adapt to fsutil SafeWriteOption hook API (fsutil plan 01 entry F4)`.)
- Tests: every `.Copy/.CopyFs/.Write(..., nil, nil)` lost its trailing
  `nil, nil`; the two arg-hook ordering tests (`copy with pre and post hooks` in
  `safe_write_write_test.go`, `with hooks` in `safe_write_writefs_test.go`) were
  rewritten to exercise struct-only hooks: multiple PreHooks run in slice order
  before the operation, multiple PostHooks after it.

## Test evidence
- `cd go-fsys-helper/fsutil && go build ./... && go vet ./... && go test -count=1 ./...` — PASS.
- `cd oci-image-copy && go build ./... && go test -count=1 ./pkg/...` — PASS
  (all pkg/cli*, pkg/imagecopy, pkg/imagecopy/fileserver, pkg/imageref,
  pkg/ocidir green) — confirms the breaking change is correctly absorbed by the
  sole production caller.
- `cd go-fsys-helper/vroot && go build ./...` — PASS (vroot does not use the hook
  params; unaffected).
