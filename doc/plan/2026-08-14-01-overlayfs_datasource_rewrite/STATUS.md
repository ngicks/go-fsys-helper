# STATUS — overlayfs rewrite #2

**State:** IMPLEMENTATION COMPLETE 2026-08-14 — all ten PLAN §6 steps landed;
pending final review. `go test ./... -count=1` green in `fsutil`, `vroot` and
`vroot-adapter/overlayfs`, and `-race` green in the latter two;
`golangci-lint run ./...` clean in `vroot` and the new module;
`GOOS=linux|windows|darwin|plan9` `go build` + `go vet` pass in both.

**Last updated:** 2026-08-14

## Decision headline (see DECISION.md)

sqlite via a custom VFS over `vroot.Fs` (`ncruces/go-sqlite3`); memory
authoritative + per-op write-through tx (no `Fs.Sync`); coarse RWMutex like
the Rust reference; canonical DataSource owns its store (lowers keep
masking); new module `vroot-adapter/overlayfs` (vroot gained a third dep,
x/sys, per D25); public
`MetadataStore` interface kept; schema v1 = path-keyed `WITHOUT ROWID`
B+Tree + `parent` adjacency, TEXT paths; lowers in mount order
(`lowers[0]` = highest); `vroot.Locker` file-extension interface added to
vroot core with shared/exclusive `LockLevel` (D22/D23) — the VFS maps
sqlite's levels onto it (SHARED→shared, RESERVED+→exclusive).

## Checklist (mirrors PLAN.md §6)

- [x] 0. Resolve all open questions with the user; DDL signed off
- [x] 1. Purge `vroot/overlayfs`; bootstrap module `vroot-adapter/overlayfs`
- [x] 2. `vroot.Locker` extension (core + osfs impl) + sqlite VFS over
       `vroot.Fs` (ncruces) + osfs/memfs round-trip
       (osfs files became wrapper `*osfs.File` — D24; vroot gained x/sys —
       D25; VFS at `internal/sqlvfs`, per-instance registration, journal
       file persists zeroed under EXCLUSIVE locking)
- [x] 3. MetadataStore interface + MetadataStoreSQLite (schema v1) +
       contract harness + bench (~50µs/op write-through on osfs/WSL2;
       SetWhiteout drops descendants + resets own opaque, SetOpaque keeps
       state — contract at metadatastore.go:13-48)
- [x] 4. DataSource: 3 constructors, canonical validation + store open,
       work-sweep, Close, read-only lower store (writable-first, EROFS/
       EPERM-classified fallback to mode=ro&immutable=1; Close closes
       store then fsys; addressing = path joins, not sub-Fs views)
- [x] 5. Node arena (coarse RWMutex), seeding, merged lookup (mount order) +
       readdir (state.go/merged.go; only per-layer mask sets live in
       memory, content read live — no children arena since vroot is
       path-addressed; caller holds the mutex across whole ops)
- [x] 6. Copy-up engine: policy interface, work-staging default, reflink
       (linux, tagged) + CopyBuffer fallback (local sync.Pool — fsutil's
       bufpool is module-internal; from-view = PathPrefixFs over canonical
       merged/; NOTE: out-of-package policies can't reach top's trees,
       §5 implemented verbatim — review item)
- [x] 7. Fs assembly (New/Option) + read path + OpenRoot sub-overlay
       (fsutil.ResolvePath over merged view; ErrTopReadOnly distinct from
       ErrNotCanonical; handle counts keyed by resolved name; sub-overlay
       Close is a no-op — only New-created root owns layers; write ops
       stubbed EROFS for step 8)
- [x] 8. Fs write path (copy-up gate, whiteout on remove, opaque mkdir,
       rename missing-whiteouts, etc.) (dir rename with live lower
       contribution → EXDEV like the reference; destination masking via
       setOpaque instead of per-entry whiteouts — strictly stronger;
       renameSubtree replays in-memory masks through the 4 store mutators;
       sharing-violation errno pair ported from old impl)
- [x] 9. Tests: acceptance, overlay-specific, concurrency `-race`, fstest
       (RunRoot over lower-seeded stacks + RunFs over overlay-written ones,
       memfs and osfs, no Option skips beyond osfs's own windows guards —
       File/Lock self-skips, the overlay's handles are not Lockers; fstest
       via ToIoFs; canonical-lower restack incl. a read-only reopen;
       concurrency incl. a counting CopyUpPolicy for exactly-once copy-up.
       No overlay source changes were needed)
- [x] 10. New-module README + vroot README pointer + cross-GOOS + lint
       (module README quick-start compile-checked; vroot's overlay section
       replaced by a pointer + old-API-replaced note; structure trees in
       `.claude/rules/local.md` and `.apm/instructions/local.instructions.md`
       updated; house `.golangci.yaml` copied into the module and
       `golangci-lint fmt` applied. Deferred: `vroot/walk_windows.go` has
       pre-existing golines drift `golangci-lint fmt` reports — untouched this
       session, invisible to `run` under GOOS=linux)

## Notes

- Reference clone: `/home/watage/gitrepo/github.com/containers/fuse-overlayfs`
  (`main` = Rust rewrite — the primary reference; C at `4759abd`). The path
  the user originally gave (`/home/watage/github.com/...`) does not exist.
- No external consumers of the current package — blast radius is the package,
  its `acceptancetest` subpackage, and `vroot/README.md`.
- Dev build uses a gitignored `go.work`; the new module must be added to it.

## Final verification (2026-08-14)

ng-reviewer (5 focus agents) + ng-test-runner gate ran over the whole
change set. Test gate: all green (both modules ± race, fsutil, vet,
cross-GOOS windows/darwin/plan9, golangci-lint 0 issues). Review verdict
was request-changes with 2 blockers, both fixed same day:

- D27 — go.mod pin: the new module builds only via go.work until vroot
  is committed/tagged and the pin bumped (release-ordering requirement,
  recorded, not fixable pre-commit).
- D28 — `DataSource` gained exported `Fsys`/`ContentPath`/`StagingPath`
  so out-of-package `CopyUpPolicy` implementations are actually possible
  (user-confirmed; proven by an external-package policy test).

Minor review findings also fixed: layer-0 self-mask provenance probe
(whiteout no longer clobbered when top holds both content and a whiteout
row — regression-tested both directions); `renameSubtree` now sets new
marks before clearing old (failure over-masks, never under-masks);
`mergedDir` re-points its inner handle after Chmod/Chown copy-up;
widen.go doc example fixed; osfs package doc documents the D24 API break.

## Open items (deferred, non-blocking)

- Release ordering per D27: tag vroot, then bump the overlayfs pin.
- vroot/README.md osfs sections are stale beyond this plan's scope
  (NewRooted/Unrooted no longer exist) — needs its own pass.
- Review-noted test gaps: failure injection on post-mutation store
  writes (memStore seam exists); OpenRoot-suboverlay write asserting
  full-path masking rows; acceptancetest File/Lock uses a single handle
  (no contention coverage); osfs windows lock path compile-verified only.
- Torn-state family documented by review: a store write failing after a
  physical mutation leaves a shadowed stale row (safe direction after the
  probe fix, but callers aren't told the op partially succeeded).
- `vroot/walk_windows.go` pre-existing golines drift.

## Next action

Commit (user-driven; ngcommit convention), honoring D27's ordering.
