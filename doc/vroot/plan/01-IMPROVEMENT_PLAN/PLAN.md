# vroot (+ vroot-adapter/sftpfs) — Improvement Plan 01

Scope: modules `github.com/ngicks/go-fsys-helper/vroot` and
`github.com/ngicks/go-fsys-helper/vroot-adapter/sftpfs`.
Origin: design review (multi-agent, supervisor-verified against source on 2026-06-13).
Baseline: all tests green before work started.
Direction constraint: nothing here may contradict `doc/vroot/plan/synthfs-redesign/PLAN.md`.

Blast radius reference: only `oci-image-copy/pkg/{imagecopy,ocidir}` and sftpfs
consume vroot (`Fs[File]`, `ReadFile`, `ReadDir`, `Widen`, `osfs.NewFs`). Nothing
in-workspace uses `Root`/`OpenRoot`/`ToIoFs`/`synthfs`/`memfs` externally.

## Entry V1 — Expand acceptance escape suite to cover all path-taking methods (security)

Evidence (verified):
- `acceptancetest/escapes.go:37-50` escape-tests ONLY `Open`, `Stat`, `Mkdir`
  against the traversal set; `Rename`/`Link`/`Symlink`/`OpenFile`/`Create`/
  `Remove`/`RemoveAll`/`Lstat`/`ReadLink`/`Chmod`-family/`MkdirAll`/`OpenRoot`
  are never escape-tested. A Root impl that forgets to confine `Rename` newname
  (write-out-of-jail) passes the suite.
- `:52-59`: symlink escape tested only via `Open` on links to non-existent
  targets — cannot distinguish "rejected for escaping" from "target missing";
  no absolute-target link, no dir-symlink as intermediate component, no chain.
- Richer fixtures already exist in `acceptancetest/fixture.go` (`RootFsys`) but
  are unused by TestRootEscapes.

Change: method-table-driven escape sub-tests over traversal set + absolute paths
+ real (existing) out-of-root symlink targets, incl. dir-symlink-as-component;
assert `vroot.ErrPathEscapes` and that PathError.Path does not leak host paths.
Test-only change. Also add dir-into-own-subtree rename vector (pairs with V2).
Size: M.

## Entry V2 — synthfs Rename: reject moving a directory into its own subtree

Evidence (verified):
- `synthfs/rename.go:40-45` re-parents with no ancestor check;
  `Rename("a", "a/b/c")` unlinks `a` and re-parents it under a node inside the
  moved subtree → detached parent-chain cycle; `resolve`'s boundary/parent
  guards never fire and `..` walks can spin (ELOOP cap counts only symlink
  hops). POSIX returns EINVAL.

Change: when target is a `*dir`, walk `newParent` ancestry up to boundary; if it
passes through target, return `WrapLinkErr("rename", ..., syscall.EINVAL)`.
Also no-op when source == destination.
Verification: new rename tests (`EINVAL` on subtree move; no hang under
`-timeout`); acceptance suite green.
Size: S.

## Entry V3 — synthfs: single ownership of file mode; uniform open-file-removal guard

Evidence (verified):
- `view_mem_buf.go:150-153`: `memHandle.Chmod` writes only `buf.mode`, while the
  node's metadata mode (under `state.mu`) is what tree `Stat` composes — a Chmod
  through an open handle is invisible to `Stat`, and `view.Stat().Mode()`
  consumers read the divergent buffer copy.
- `disableOpenFileRemoval` refcount guard exists only in `remove.go:23`;
  `_ = f.view.Close()` (or `nd.view.Close()`) appears unguarded in 7 sites:
  `add.go:69,189,213,219`, `remove.go:28,80`, `rename.go:36` — Rename-over,
  RemoveAll, and Add-override close views out from under open handles.

Change: route handle Chmod to the node (node owns mode/mtime; view authoritative
for Size only, as already documented); extract one unlink-and-close helper that
applies the refcount guard uniformly (Rename-over and RemoveAll honor it; add
that to docs).
Verification: new tests for handle-Chmod visibility and guard uniformity;
`go test ./synthfs/... -race`; acceptance suite green.
Size: M.

## Entry V4 — WalkDir: fix sibling-loop truncation; document escape-safety bounds

Evidence (verified):
- `walk.go:181-188`: on a child `Lstat` error, after `fn` returns nil the code
  `return err` (nil) — silently truncating the remaining siblings (stdlib
  continues). The `info != nil && info.IsDir()` guard at `:184` is dead (`info`
  is nil on that path).
- `WalkDir` accepts any `Fs`; with `ResolveSymlink:true` it is escape-safe only
  over a `Root`. `Sub` (`funcs.go`) falls back to `PathPrefixFs`, which is
  documented as not a security boundary, while Sub's own doc implies blocking.

Change: continue the sibling loop when `fn` swallows the Lstat error; remove the
dead guard; tighten docs on `WalkDir`/`Sub`/`PathPrefixFs` (lexical-only, escape
safety requires Root). New `WalkDirRoot` API NOT added (see DECISION.md D8).
Verification: sibling-continuation test (one unreadable child must not stop the
walk); existing walk tests green.
Size: S.

## Entry V5 — internal/paths: clean inputs, never emit empty/.. tokens

Evidence (verified):
- `internal/paths/head_tail.go:11-30`: `PathFromHead("/a/b")` yields a leading
  `""` (leading separator at index 0).
- `:32-49`: `PathFromTail` never cleans: `"a/../b"` yields `..`-bearing tokens,
  `"a//b"` yields `"a/"`. Latent footgun for any future confinement use.

Change: Clean input in both; define and test behavior for absolute inputs,
`"."`, `".."`, trailing slash, doubled separators; table test.
Verification: new table test; synthfs (sole caller, pre-normalized input)
behavior unchanged; full module tests green.
Size: S.

## Entry V6 — sftpfs: honor the vroot error convention; fix package doc

Evidence (verified):
- `sftpfs.go:1` claims `vroot.Unrooted` — type does not exist (it implements
  `vroot.Fs`), and the wording invites a false confinement assumption.
- `MkdirAll` returns the raw `sftp` client error (no `mapSftpErr`, no
  `WrapPathErr`) so `errors.Is(err, fs.ErrExist)` fails — while `Mkdir` maps it;
  oci-image-copy relies on MkdirAll idempotency (remote.go, push.go,
  fsocidirs.go). `Lchown`/`Link`/`Rename`/`Stat`/others return raw client errors
  bypassing the documented `*fs.PathError`/`*os.LinkError` convention.

Change: wrap every client-call result via `WrapPathErr(op, name, mapSftpErr(err))`
(`WrapLinkErr` for Link/Rename); fix package doc to "implements vroot.Fs
(escapable; not symlink-confined, like osfs.Fs)". Add typed-error and
MkdirAll-idempotency assertions to the sftpfs test.
Verification: sftpfs tests green; oci-image-copy tests green.
Size: S-M.

## Explicitly not planned
- `vroot.File` capability split: out of proportion, conflicts with the
  *os.File-shaped contract; skipped.
- osfs `translateEscape` string-matching of the std sentinel: documented
  watch-item, no cleaner pre-Go-API option.
- synthfs OpenRoot boundary liveness / MkdirAll errno nuances / chmodApply
  setuid bits / dir-handle pagination: folded into V3's hardening only where
  trivially adjacent, otherwise deferred.
