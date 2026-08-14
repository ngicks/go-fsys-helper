# STATUS — overlayfs rewrite #2

**State:** PLAN FINALIZED 2026-08-14 — all open questions resolved over four
AskUserQuestion rounds (D13–D21); traceability gate walked (every decision
clause maps to a PLAN §6 step; both IDEA use cases covered; no HANDOFF.md —
nothing left behind). Not started on code.

**Last updated:** 2026-08-14

## Decision headline (see DECISION.md)

sqlite via a custom VFS over `vroot.Fs` (`ncruces/go-sqlite3`); memory
authoritative + per-op write-through tx (no `Fs.Sync`); coarse RWMutex like
the Rust reference; canonical DataSource owns its store (lowers keep
masking); new module `vroot-adapter/overlayfs` (vroot stays 2-dep); public
`MetadataStore` interface kept; schema v1 = path-keyed `WITHOUT ROWID`
B+Tree + `parent` adjacency, TEXT paths; lowers in mount order
(`lowers[0]` = highest); `vroot.Locker` file-extension interface added to
vroot core with shared/exclusive `LockLevel` (D22/D23) — the VFS maps
sqlite's levels onto it (SHARED→shared, RESERVED+→exclusive).

## Checklist (mirrors PLAN.md §6)

- [x] 0. Resolve all open questions with the user; DDL signed off
- [ ] 1. Purge `vroot/overlayfs`; bootstrap module `vroot-adapter/overlayfs`
- [ ] 2. `vroot.Locker` extension (core + osfs impl) + sqlite VFS over
       `vroot.Fs` (ncruces) + osfs/memfs round-trip
- [ ] 3. MetadataStore interface + MetadataStoreSQLite (schema v1) +
       contract harness + bench
- [ ] 4. DataSource: 3 constructors, canonical validation + store open,
       work-sweep, Close, read-only lower store
- [ ] 5. Node arena (coarse RWMutex), seeding, merged lookup (mount order) +
       readdir
- [ ] 6. Copy-up engine: policy interface, work-staging default, reflink
       (linux, tagged) + CopyBuffer fallback
- [ ] 7. Fs assembly (New/Option) + read path + OpenRoot sub-overlay
- [ ] 8. Fs write path (copy-up gate, whiteout on remove, opaque mkdir,
       rename missing-whiteouts, etc.)
- [ ] 9. Tests: acceptance, overlay-specific, concurrency `-race`, fstest
- [ ] 10. New-module README + vroot README pointer + cross-GOOS + lint

## Notes

- Reference clone: `/home/watage/gitrepo/github.com/containers/fuse-overlayfs`
  (`main` = Rust rewrite — the primary reference; C at `4759abd`). The path
  the user originally gave (`/home/watage/github.com/...`) does not exist.
- No external consumers of the current package — blast radius is the package,
  its `acceptancetest` subpackage, and `vroot/README.md`.
- Dev build uses a gitignored `go.work`; the new module must be added to it.

## Next action

Implementation, starting at step 1 (awaiting the user's go-ahead).
