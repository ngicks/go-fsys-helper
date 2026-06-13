# SUMMARY — V2: synthfs Rename subtree-cycle rejection

## What changed

`vroot/synthfs/rename.go`:

1. **Reject moving a directory into its own subtree.** After resolving `target`
   (the node being moved) and `newParent`, if `target` is a `*dir` and it is an
   ancestor-or-self of `newParent`, return
   `WrapLinkErr("rename", old, new, syscall.EINVAL)` — matching POSIX
   rename(2). Previously `Rename("a","a/b/c")` unlinked `a` and re-parented it
   under a node *inside the moved subtree*, detaching the parent chain into a
   cycle; later `..` walks would never reach the boundary and could spin (the
   ELOOP cap counts only symlink hops, not parent hops).

   New helper `(*Root).isAncestorOrSelf(anc, start *dir) bool` walks the
   `parent` chain from `start` up to and including `r.boundary`.

2. **Source == destination is a no-op.** When `newParent == oldParent &&
   newBase == oldBase`, return nil before any mutation. This also stops the
   subtree guard from mis-firing on `Rename("a","a")`.

The ancestry check and self no-op run BEFORE the existing overwrite/removal
logic, so a rejected rename never mutates the tree.

## Acceptance vector (the deferred plan-01 V1 vector lands here)

`vroot/acceptancetest/rename.go` `TestRenameUnix` gains a
"directory into own subtree" subtest asserting `syscall.EINVAL` for both a deep
descendant (`mvdir -> mvdir/inner/moved`) and a direct child
(`mvdir -> mvdir/child`), then asserts the tree is still intact/walkable. This
runs against every OsUnix Root/Fs (osfs.Fs, osfs.Root, synthfs) — verified that
plain `os.Rename` (osfs.Fs), `*os.Root.Rename` (osfs.Root), and synthfs all
return EINVAL here.

A "rename onto self is a no-op" acceptance vector was deliberately NOT added: it
is not portable — Linux rename(2) of a *directory* onto itself returns EEXIST
through plain `os.Rename` (the osfs.Fs path), while `*os.Root.Rename` and
synthfs treat it as a no-op. synthfs's no-op guarantee is instead covered by the
synthfs-specific test below.

## synthfs-specific tests

New `vroot/synthfs/rename_test.go`:
- `TestRenameDirIntoOwnSubtree` — EINVAL for deep descendant / direct child /
  into-self-as-parent, then a `..`-walking Stat to prove the tree did not become
  a cycle (must complete within `-timeout`).
- `TestRenameOntoSelfNoop` — `Rename(d,d)` and `Rename(d/f.txt,d/f.txt)` succeed
  and leave contents in place.
- `TestRenameDirMoveStillWorks` — a legitimate move to a SIBLING subtree
  (`src/inner -> dst/inner`) still succeeds (guards against the subtree check
  over-firing).

## Test evidence

```
cd go-fsys-helper/vroot
go build ./...                                   # ok
go vet ./...                                      # ok
go test ./... -race -timeout 120s                 # ok (no hang; race clean)
go test ./synthfs/ -run Rename -race -timeout 120s -v  # all PASS
```

oci-image-copy consumer (synthfs is consumed transitively):
```
cd oci-image-copy
go build ./...        # ok
go test ./pkg/imagecopy ./pkg/ocidir   # ok
```
(Note: `pkg/cli/ssh` showed a transient build failure from a concurrent agent's
unrelated in-flight edit — unused imports in ssh_test.go. It does not consume
vroot; the vroot consumers pass.)

## Implementation bugs found

The pre-V2 synthfs silently corrupted the tree on `Rename("a","a/b/c")`
(returned nil, detached parent chain). Fixed here. No new bugs found.

## Deviation from plan

The plan specifies `WrapLinkErr("rename", ..., syscall.EINVAL)` — implemented
exactly. The acceptance "rename onto self" assertion was dropped as non-portable
(see above); the synthfs no-op behavior is still tested package-locally.

## Files changed

- `vroot/synthfs/rename.go` (subtree guard + self no-op + isAncestorOrSelf)
- `vroot/synthfs/rename_test.go` (new)
- `vroot/acceptancetest/rename.go` (dir-into-own-subtree EINVAL vector)
