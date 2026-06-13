# SUMMARY — V1: expand acceptance escape suite (all methods, real targets)

## What changed

Rewrote `vroot/acceptancetest/escapes.go` from a 3-method (`Open`/`Stat`/`Mkdir`)
spot-check into a method-table-driven escape suite that exercises **every**
path-taking `vroot.Root` method against:

- the **syntactic-escape** traversal set (`..`, `../`, `../sibling`, `../outside`,
  `sub/../..`, `sub/../../outside`), and
- **symlink-driven escapes** through real out-of-root targets (file symlink, dir
  symlink as final AND as intermediate component, absolute-target link, and a
  two-hop chain).

Method coverage now includes (previously untested): `OpenFile`,
`OpenFile(O_CREATE)`, `Create`, `Lstat`, `ReadLink`, `MkdirAll`, `Remove`,
`RemoveAll`, `Chmod`, `Chown`, `Lchown`, `Rename` (both oldname and newname /
write-out-of-jail), `Link` (newname), `Symlink` (newname), and `OpenRoot`.

Subtest count per Root implementation went from ~14 to **184**.

Each rejection asserts `vroot.ErrPathEscapes` AND that the surfaced
`*fs.PathError`/`*os.LinkError` path fields are not absolute (a confined Root must
echo the caller's relative name, never leak a host-side resolved path).

### Real out-of-root targets (the "caught actually reading them" requirement)

- Added `SetupRoot.SetupExternal func(t *testing.T)` (in `acceptancetest.go`): an
  optional hook for filesystem-backed Roots to materialize the genuine
  out-of-root files the escape symlinks point at.
- `osfs/osfs_test.go` `TestRoot` now roots the Fs at `base/root` and creates real
  sibling files `base/outside` (content `OUT-OF-ROOT-SECRET`) and
  `base/outsidedir/inside` (`OUT-OF-ROOT-SECRET-2`). The escape symlinks
  (`escapelink -> ../outside`, `escapedir -> ../outsidedir`, …) therefore resolve
  to real files: a confinement regression that *followed* a link would read those
  secret bytes (a visible success) instead of a benign `ErrNotExist`.
- synthfs/memfs are pure in-memory: nothing exists outside the root, so
  `SetupExternal` is left nil and escapes are rejected lexically.

### Supporting change

`run.go`: replaced the `Setup[F, R](s)` struct conversion (which only compiled
while `SetupRoot` and `Setup` had identical field layouts) with an explicit
`asFsSetup` projector, since `SetupRoot` now carries the extra `SetupExternal`
field.

## Method-table design notes (correctness, not arbitrary)

- **Lstat / ReadLink are NOT tested against symlink-target escapes.** They operate
  on the link itself; a symlink stored *inside* the root that merely points
  outside is legitimate (verified: both osfs and synthfs return the link info /
  target with no error). They ARE tested against path-component (`..`) escapes,
  where they correctly return `ErrPathEscapes`.
- **Remove / RemoveAll are split into `symlinkUnlinkMethods`** and run only over
  *intermediate*-component symlink targets (`escapedir/inside`). POSIX unlink does
  not follow a *final* symlink, so `Remove("escapelink")` legitimately unlinks the
  in-root link (not an escape) — and doing so on the shared Root would also
  destroy fixtures for later subtests.

## Implementation bugs the expanded suite exposed

**None.** Both `osfs.Root` and `synthfs.Root` confine all path-taking methods
correctly: every expanded vector returns `vroot.ErrPathEscapes` with a relative
(non-leaking) path field. The expanded suite hardened the contract without
finding a real escape in either implementation.

## Deviation from plan (order note)

The plan's V1 "dir-into-own-subtree rename" acceptance vector is **deferred to
V2** (per the PLAN/STATUS order note): it fails against synthfs until the V2 fix
lands (synthfs currently silently corrupts the tree on `Rename("a","a/b/c")`
instead of returning `EINVAL`). It will be added to the acceptance/synthfs tests
in the V2 commit. All other V1 vectors are live now.

## Test evidence

```
cd go-fsys-helper/vroot
go build ./...   # ok
go vet ./...     # ok
go test ./...    # ok (vroot 10.8s, memfs 1.5s, osfs 5.6s, synthfs 3.0s)

go test ./osfs/    -run 'TestRoot/Escapes' -v  # 184 escape subtests, all PASS
go test ./synthfs/ -run 'TestRoot/Escapes' -v  # 184 escape subtests, all PASS
```

oci-image-copy consumer (acceptancetest is test-only, but verified):
```
cd oci-image-copy
go build ./...        # ok
go test ./pkg/...     # ok
```

## Files changed

- `vroot/acceptancetest/escapes.go` (rewrite)
- `vroot/acceptancetest/acceptancetest.go` (`SetupRoot.SetupExternal` field)
- `vroot/acceptancetest/run.go` (`asFsSetup` projector)
- `vroot/osfs/osfs_test.go` (real out-of-root escape targets)
