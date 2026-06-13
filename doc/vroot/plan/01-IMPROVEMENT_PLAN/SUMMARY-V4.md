# SUMMARY — V4: WalkDir sibling-loop fix + escape-safety docs

## Bug fix (sibling-loop truncation)

`vroot/walk.go` (the child-Lstat-error branch of `walkDir_`): when `fsys.Lstat`
on a child errored, the code called `fn` and then unconditionally
`return err` — so if `fn` swallowed the error (returned nil), the function
returned nil from the *middle of the sibling loop*, silently truncating the
directory's remaining entries. `filepath.WalkDir` continues in this case. The
`if err == SkipDir && info != nil && info.IsDir()` guard was dead: `info` is
always nil on the Lstat-error path.

Fix: hand the error to `fn`; if `fn` swallows it (nil) or returns `SkipDir`
(there is nothing to descend into on a failed entry anyway), `continue` to the
next sibling. Any other error — including `SkipAll` — propagates and terminates,
matching stdlib. The dead guard is removed.

## Docs (escape-safety bounds; D8 — no WalkDirRoot API)

Per decision D8, NO `WalkDirRoot` API was added (zero in-workspace WalkDir
consumers). Instead the escape-safety bounds are documented:

- `WalkDir` (walk.go): a new doc comment spells out the control flow (SkipDir /
  SkipAll / Lstat-error continuation) and states that with `ResolveSymlink` the
  walk is escape-safe ONLY over a confined `Root` — confinement is enforced by
  `fsys`, not by WalkDir; over an escapable `Fs` a symlink may resolve outside
  the tree and be followed.
- `Sub` (funcs.go): documents that the `NewPathPrefixFs` fallback is lexical-only
  and NOT a security boundary (does not confine symlink targets, TOCTOU); for a
  real boundary derive from a `Root`.
- `PathPrefixFs` (pathprefix.go): expanded doc — lexical "../" blocking only, no
  symlink-target confinement, TOCTOU; use a `Root` for confinement.

## Tests

New `vroot/walk_sibling_test.go`:
- `TestWalk_SiblingContinuesAfterLstatError` — an `lstatFailFs` wrapper injects an
  Lstat error for a middle sibling (`c.txt`) among five files; the walk must still
  visit `a/b/d/e` (proving entries after the failed one are not truncated) and the
  error is delivered to fn exactly once.
- `TestWalk_SiblingSkipAllAfterLstatError` — `SkipAll` from the error callback
  still terminates the whole walk (no later siblings visited).

All existing walk tests (`TestWalk_Rooted_*`, `TestWalk_Unrooted_*`) remain green.

## Test evidence

```
cd go-fsys-helper/vroot
go build ./...                          # ok
go vet ./...                            # ok
go test ./...                           # ok
go test . -run TestWalk -v              # all PASS (new + existing)
```

oci-image-copy consumer:
```
cd oci-image-copy
go build ./...        # ok
go test ./pkg/...     # ok
```

## Implementation bugs found

The sibling-loop truncation itself (above) — a real bug: a single unreadable
entry silently dropped all following siblings from a walk. Fixed.

## Deviation from plan

None. D8 honored (no WalkDirRoot API; docs instead).

## Files changed

- `vroot/walk.go` (sibling continuation fix + WalkDir doc)
- `vroot/funcs.go` (Sub escape-safety doc)
- `vroot/pathprefix.go` (PathPrefixFs escape-safety doc)
- `vroot/walk_sibling_test.go` (new)
