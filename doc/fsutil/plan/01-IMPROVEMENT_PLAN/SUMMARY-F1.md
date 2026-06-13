# SUMMARY — F1 (with F6 folded in)

## F1 — WrapPathErr/WrapLinkErr must not mutate the caller's error

### What changed
`fsutil/wrap_path_err.go`: `WrapPathErr` and `WrapLinkErr` no longer rewrite
`Op`/`Path` (and `Old`/`New`) on the caller's `*fs.PathError`/`*os.LinkError`.
Instead they allocate a shallow copy (`cp := *pathErr`), merge the non-zero
`op`/`path` (`old`/`new`) fields into the copy, and return a pointer to the copy.

The field-merge semantics are preserved exactly (D7): non-empty arguments still
override, empty arguments still keep the original field value. The only
observable difference is that the returned pointer is now distinct from the
input and the input value is never mutated.

### Why
`wrap_path_err.go` previously rewrote `Op`/`Path` on the SAME struct and returned
it. Any error value that is shared (sentinel-ish, stored, or observed by another
goroutine) was silently rewritten under the caller, and the helper was
non-idempotent across calls. fsutil is consumed by `vroot` (`WrapPathErr` at 40+
sites in synthfs, `ResolveSymlink` in walk), where this aliasing is a latent
correctness hazard. Decision D7 chose copy-on-write over double-wrapping
(double-wrapping would change rendered messages at 40+ vroot call sites).

### Tests
Added to `fsutil/wrap_path_err_test.go`:
- `TestWrapPathErr_doesNotMutateInput`: input `*fs.PathError` unchanged after the
  call; returned pointer is distinct; merged fields correct; wrapped `Err`
  preserved.
- `TestWrapPathErr_emptyFieldsPreserveOriginal`: empty op/path leave the copy's
  fields untouched and do not mutate the input.
- `TestWrapLinkErr_doesNotMutateInput` and
  `TestWrapLinkErr_emptyFieldsPreserveOriginal`: same assertions for
  `*os.LinkError`.

### Test evidence
- `cd go-fsys-helper/fsutil && go build ./... && go vet ./... && go test ./...` — PASS.
- `go test -run 'TestWrapPathErr|TestWrapLinkErr' -v ./...` — all 4 new tests
  PASS, existing `TestWrapPathErr`/`TestWrapLinkErr` PASS.
- `cd go-fsys-helper/vroot && go build ./... && go test ./...` — PASS (synthfs,
  memfs, osfs, vroot all green), confirming the heavy vroot consumer is
  unaffected by the behavior change.

## F6 — fs.ModeSymlink consistency in resolve_symlink.go (folded into F1 commit)

### What changed
`fsutil/resolve_symlink.go`: the lone `info.Mode()&os.ModeSymlink == 0` check in
`ResolveSymlink` now uses `fs.ModeSymlink` (same bit), matching the rest of the
file. The `os` import — previously needed only for that one reference — was
removed.

NOT taken (per D5): unexporting `ResolvePath`/`SyncHook`/`ChmodMask`. This is a
published module with pinned external consumers; only the cosmetic constant fix
is in scope.

### Why
The file mixed `os.ModeSymlink` and `fs.ModeSymlink` for the same bit; `os` was
imported solely for that. F6 is XS and the plan permits folding it into the F1
commit, which is done here.

### Test evidence
Covered by the same fsutil and vroot runs above (build/vet/test all green;
`go vet` confirms no unused import).
