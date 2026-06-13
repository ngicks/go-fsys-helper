# SUMMARY — V3: synthfs single mode-ownership + uniform open-file-removal guard

## D10 — node owns mode/mtime; views own bytes/size

`vroot/synthfs/view_mem_buf.go`: `memHandle.Chmod` no longer writes a
buffer-local `buf.mode`. It is now a no-op, because the tree node owns the mode
(the view is authoritative only for content/Size). `*fileHandle.Chmod` already
routes Chmod to the node (`chmodApply` on `h.node.metadata`) and never delegates
to the inner view, so a Chmod through an open handle is now consistently visible
to both the tree's `Stat` and the handle's `Stat`. Pre-V3 the buffer-local write
was a latent divergence: anything reading `view.Stat().Mode()` would have seen a
mode the tree never composed.

## D9 — uniform DisableOpenFileRemoval guard

New file `vroot/synthfs/unlink.go` centralizes the two concerns every unlink path
shares:

- `(*Root).guardOpenRemoval(n node) error` — pure pre-flight check returning the
  sharing-violation error when `DisableOpenFileRemoval` is set and `n` (or, for a
  directory, any descendant regular file) still has open handles.
- `hasOpenHandle(n node) bool` — recursive refcount probe.
- `closeUnlinkedView(n node)` — the single place a `*file`'s view is closed on
  unlink.

The previously-unguarded close sites now route through these helpers, making the
guard uniform:

- `remove.go` Remove — converted to `guardOpenRemoval` + `closeUnlinkedView`.
- `remove.go` RemoveAll — NEW: pre-flights `guardOpenRemoval` over the WHOLE
  subtree before `removeSubtree` runs, so a sharing violation deep in the tree
  aborts atomically (no partial deletion). `removeSubtree` now closes via
  `closeUnlinkedView`.
- `rename.go` Rename-over — NEW: guards the destination before unlinking it.
- `add.go` AddFile-override, AddFS symlink-override, AddFS file-override — NEW:
  guard the existing node before unlinking it (the AddFS file path closes the
  freshly-built incoming view before aborting so it is not leaked). The AddFS
  Keep-discard path closes only the INCOMING view (not an unlink of an existing
  open file), so no guard applies there.

### Scope boundary (documented)

`materializeDir`'s directory-override path (AddFS overriding an existing dir with
a subtree) still prunes via `removeSubtree` without the guard. It returns a
`*dir` (no error channel), so threading a guard error through it and
`addFSWalk` would be a disproportionate signature change for a rare ingest-time
case. This matches the plan's named close sites (add.go:69/189/213/219, not
materializeDir) and is called out here. `removeSubtree` was already unguarded for
this path pre-V3, so there is no behavior regression.

## Tests

New `vroot/synthfs/ownership_guard_test.go`:
- `TestHandleChmodVisibleToStat` — handle Chmod visible to tree Stat AND handle
  Stat; size still read live from the view.
- `TestHandleChmodThenTreeChmod` — tree and handle Chmod hit the same field.
- `TestGuardRemoveOpenFile` — baseline (Remove already honored the guard).
- `TestGuardRemoveAllOpenFile` — NEW: RemoveAll honors the guard and rejects the
  whole subtree atomically (every entry survives).
- `TestGuardRenameOverOpenFile` — NEW: Rename over an open destination honors the
  guard; both src and dst survive.
- `TestGuardAddFileOverrideOpenFile` — NEW: AddFile override honors the guard.
- `TestGuardOffAllowsUnlink` — with the option off, the same ops succeed with an
  open handle.

## Test evidence

```
cd go-fsys-helper/vroot
go build ./...                                          # ok
go vet ./...                                            # ok
go test ./... -race -timeout 120s                       # ok (race clean)
go test ./synthfs/ -run 'TestHandleChmod|TestGuard' -race -timeout 120s -v  # all PASS
```

oci-image-copy consumer:
```
cd oci-image-copy
go build ./...        # ok
go test ./pkg/...     # ok
```

## Implementation bugs found

- Pre-V3, RemoveAll / Rename-over / Add-override closed a FileView out from under
  still-open handles even with DisableOpenFileRemoval set (the option only
  affected Remove). Fixed: the guard is now uniform.
- `memHandle.Chmod` wrote a divergent buffer-local mode. Fixed: it delegates
  (no-op); the node is the sole owner of mode.

## Deviation from plan

Within plan scope. The only documented narrowing is the materializeDir
dir-override path (see Scope boundary above), consistent with the plan's named
close sites.

## Files changed

- `vroot/synthfs/unlink.go` (new — guard + close helpers)
- `vroot/synthfs/remove.go` (Remove + RemoveAll route through helpers)
- `vroot/synthfs/rename.go` (Rename-over guard)
- `vroot/synthfs/add.go` (AddFile/AddFS override guard)
- `vroot/synthfs/view_mem_buf.go` (memHandle.Chmod no-op)
- `vroot/synthfs/ownership_guard_test.go` (new)
