# DECISION log — overlayfs rewrite #2

Numbered `D<n>`. Entries D1–D12 are settled (user-directed or routine judgment
calls, noted as such). OQ stubs at the bottom fill in as PLAN.md §9 resolves.

## Settled

- **D1 — Plan location (revised by the user).**
  `doc/plan/2026-08-14-01-overlayfs_datasource_rewrite/`, per the ngplan
  skill convention (`./doc/plan/<YYYY-MM-DD>-NN-<snake_case_slug>`). The
  initial judgment call placed it at
  `doc/vroot/plan/02-overlayfs-datasource-rewrite/` following the older
  plans' layout; the user corrected this — the skill convention wins.
  (Plans 00/01 remain where they are.)
- **D2 — IDEA phase skipped.** User-directed: "Skip IDEA phase since this is
  clearly library and use case is very clear to me." IDEA.md is a stub
  recording the stated intent only.
- **D3 — DataSource is one concrete struct with two constructors** (plain
  `vroot.Fs`; canonical `work/`+`merged/`), per the user's wording ("It will
  have 2 constructor for DataSource"). `vroot.Fs` stays the extension point
  for custom backends; no DataSource interface, no dlopen-style plugin layer.
- **D4 — Top must be canonical; `New` returns an error.** Non-canonical top →
  `ErrNotCanonical`; store-load failure → error. Supersedes plan-01's D10
  (errorless `New` degrading silently).
- **D5 — A canonical lower reads `merged/` only**; its `work/` is ignored.
  (Whether its metadata DB is consulted is OQ5.)
- **D6 — No masking artifacts as files, ever** (user-directed: "There's not
  whiteout file in data source"). No `.wh.*`, no char-0:0 devices, no
  `.wh..wh..opq`, no opaque xattrs. Whiteout/opaque live only in the
  metadata store.
- **D7 — Opaque on every freshly created directory**, following the
  reference (`ovl_mkdir` passes `set_opaque=true` unconditionally;
  C `main.c:5227` @ 4759abd). Harmless when no lower dir exists (a visible
  lower dir makes Mkdir fail EEXIST anyway); makes `rm -rf && mkdir` mask
  lower content without a special recreate-over-whiteout rule. Parent dirs
  materialized for copy-up are NOT opaque.
- **D8 — `work/` is swept at `New`**, mirroring the reference's
  empty-workdir-on-mount. `work/` is overlay-private; nothing in it survives
  a restart.
- **D9 — Copy-data cascade is exactly reflink → `io.CopyBuffer`**
  (user-directed). Reflink = Linux `unix.IoctlFileClone` when both files
  expose a valid `Fd()` (vroot's `^uintptr(0)` sentinel = not os-backed →
  skip). No sendfile, no copy_file_range tier (the reference's copy-up does
  not use copy_file_range either).
- **D10 — No xattr support.** `vroot.Fs` has no xattr surface, so the
  reference's xattr copy / `xattr_permissions` emulation / opaque-xattr
  machinery has nothing to attach to. Non-goal.
- **D11 — Lowers carry no metadata parameter by default.** Plan-01's
  accepted-but-ignored `Layer.meta` for lowers is dropped; only OQ5 can
  reintroduce lower-side metadata (as the canonical DataSource's own frozen
  store).
- **D12 — SUPERSEDED by D20.** (Was: keep the vroot last-wins lower
  ordering. The round-3 answer flipped it to mount order.)

## Resolved with the user (round 1, 2026-08-14)

- **D13 (OQ1) — sqlite via a custom VFS over `vroot.Fs`, driver
  `github.com/ncruces/go-sqlite3`.** *User chose this over the recommended
  OS-path/DSN + modernc default.* The DB `meta.sqlite3` lives inside the
  canonical DataSource's fs beside `work/`+`merged/`, whatever the backend
  (osfs, memfs, sftpfs…). Rejected: explicit OS-path constructor (+ modernc
  or ncruces) — would exclude fully-virtual tops from durable metadata and
  leak an OS-path concept into an otherwise vroot-closed API. Consequences:
  ncruces+wazero dependency; VFS implementation work; journal mode must
  avoid shm (rollback journal, or WAL only with `locking_mode=EXCLUSIVE`).
- **D14 (OQ2) — memory authoritative + per-op write-through transactions.**
  In-memory node state is seeded from sqlite at `New`; each masking change
  commits before the op returns; lookups never touch sqlite. `Fs.Sync()` is
  dropped from the API. Rejected: sqlite-on-the-read-path; plan-01-style
  write-behind batching + `Sync()`.
- **D16 (OQ4) — coarse lock, reference-style.** One RWMutex over node
  arena + store writes; data I/O on already-open handles never takes it;
  copy-up dedup = write-lock + "already in top?" re-check (no singleflight).
  Verified reference behavior: Rust `overlay.rs:82-113`, `1211-1230`.
  Rejected: plan-01's per-node locks + lock ordering + singleflight.
- **D17 (OQ5) — the canonical DataSource owns its MetadataStore.**
  `NewDataSourceCanonical` opens/creates the store itself; writable as top,
  read-only as a lower, and a lower's whiteouts keep masking deeper layers —
  a stopped overlay restacks losslessly. `Option.Metadata` does not exist.
  Rejected: top-only store via `Option`/`New` param (restacking would forget
  deletions).

## Resolved with the user (round 2, 2026-08-14)

- **D15 (OQ3) — new module `vroot-adapter/overlayfs`.** User's own answer:
  "Own module. Move it to vroot-adapter/overlayfs." `vroot/overlayfs` is
  deleted with no in-module replacement; the rewrite lands as module
  `github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs` (requiring
  `vroot`, `fsutil`, `ncruces/go-sqlite3`, `golang.org/x/sys`), keeping the
  `vroot` module at 2 deps. Rejected: absorbing the dep into `vroot`;
  splitting only the store out.
- **D18 (OQ7) — public `MetadataStore` interface kept.** *User chose this
  over the recommended fully-internal default.* Shape: `Load` + 4 setters +
  `Close`; setters durable on return (no `Flush`, per D14). Plus
  `NewMetadataStoreSQLite(fsys, name)` and
  `NewDataSourceCanonicalStore(fsys, store)`; the
  `acceptancetest.RunMetadataStore` contract harness is rewritten in the new
  module. Rejected: internal-only store.

## Resolved with the user (round 3, 2026-08-14)

- **D19 (OQ8) — hybrid path-keyed trie schema.** User's note: "mix of
  adjacency list and full path. Faster search by B+Tree index range query
  but a bit more complex." Realized as: `node` table keyed by full path,
  `WITHOUT ROWID` (clustered B+Tree — subtree ops are index range scans
  `path >= p||'/' AND path < p||'0'`), plus an indexed adjacency `parent`
  column for direct-children queries. Journal: no-shm rollback journal,
  exclusive locking (implementation default). Rejected: pure component-level
  adjacency list (recursive CTEs for subtrees); pure materialized path with
  no adjacency column. Final DDL signed off in round 4 → D21.
- **D20 (OQ9) — layer ordering is MOUNT ORDER.** `lowers[0]` = highest-
  priority lower, like `lowerdir=a:b:c`; resolution `top → lowers[0] →
  lowers[1] → …`. Supersedes D12. Rejected: vroot's old last-wins
  convention; a dual-spelling API.

## Resolved with the user (round 4, 2026-08-14)

- **D21 — DDL v1 signed off; path columns are `TEXT`.** User challenged the
  BLOB draft ("why BLOB? TEXT type on SQLite3 not a utf8 array?"); answered
  that sqlite never validates TEXT as UTF-8 and default BINARY collation is
  bytewise (so range scans are identical), and the user chose TEXT for
  tooling-friendliness. Non-UTF-8 names round-trip regardless; they merely
  render as mojibake in tools. Rejected: BLOB columns. Schema v1 is PLAN §5's
  DDL verbatim; journal = no-shm rollback journal + exclusive locking.

**All open questions resolved. Plan finalized 2026-08-14.**

## Post-finalization additions (user-directed, 2026-08-14)

- **D22 — `vroot.Locker` extension interface; VFS locks through it.**
  User-directed: add a go-billy-style optional extension on `vroot.File` —
  `Lock() error` / `Unlock() error` — with a doc WARNING that Lock may
  switch the file into non-blocking mode. Implemented on osfs (`flock` /
  `LockFileEx`); optional elsewhere. The sqlite VFS maps sqlite's 5 lock
  levels onto this single exclusive lock the same way sqlite's own "flock"
  locking style does (any level ≥ SHARED → the one exclusive lock;
  `CheckReservedLock` from local state), which is sufficient BECAUSE the
  design already runs `locking_mode=EXCLUSIVE` (D13/D19); backends without
  `Locker` fall back to in-process bookkeeping (sole-user precondition
  still holds). Known limits, accepted: no cross-process shared readers
  (non-goal), and no try/timeout form so contended opens block rather than
  return `SQLITE_BUSY` — contention would mean two overlays on one top,
  which the design forbids. Rejected: putting Lock/Unlock on the `File`
  interface itself (breaks every implementation); a multi-level lock API
  mirroring sqlite (overfit to one consumer). *Amended by D23: the
  no-levels shape was revised the same day.*
- **D23 — `Locker` carries a two-level `LockLevel` (shared/exclusive).**
  User-directed follow-up: POSIX (flock `LOCK_SH`/`LOCK_EX`, fcntl
  `F_RDLCK`/`F_WRLCK`) and Windows (`LockFileEx` ± `LOCKFILE_EXCLUSIVE_LOCK`)
  both natively distinguish shared vs exclusive, so the interface becomes
  `Lock(level LockLevel) error` / `Unlock() error` with
  `LockShared`/`LockExclusive`. Re-calling Lock converts the held lock
  (documented as possibly non-atomic — e.g. Windows must unlock-then-relock).
  The non-blocking-mode WARNING stays. VFS mapping updated: sqlite `SHARED` →
  `LockShared`; `RESERVED`+ → `LockExclusive`; downgrade via
  `Lock(LockShared)`; release via `Unlock()` — strictly more faithful than
  D22's flock-style collapse (cross-process shared readers become possible
  in principle), while `locking_mode=EXCLUSIVE` still makes conversion
  windows moot in our own use. Rejected: sqlite's 5-level enum (overfit;
  not expressible by whole-file OS locks anyway); keeping the level-less
  shape (cannot express shared locks the OSes already offer).

## Implementation-time decisions (2026-08-14, run start)

- **D24 — osfs files become a wrapper type `osfs.File` (user-confirmed).**
  Step 2a found osfs's file type is the bare stdlib `*os.File`
  (`vroot.Fs[*os.File]`), which cannot gain `Lock`/`Unlock` methods —
  "implement Locker on osfs files" was unimplementable as planned. Chosen: a
  wrapper `osfs.File struct{ *os.File }`; `Fs`/`Root` type parameters change
  to `*osfs.File`. Public API break in osfs, but the only route satisfying
  D22's assert-`vroot.Locker`-via-type-switch contract verbatim (widening
  passes concrete files through, so the assertion still hits). Rejected: a
  generic `Fd()`-based lock helper in vroot core (no API break, but amends
  D22's contract).
- **D25 — vroot gains a direct `golang.org/x/sys` dependency
  (user-confirmed).** Stdlib `syscall` has no `LockFileEx`/`UnlockFileEx` on
  windows (verified; they live only in `internal/syscall/windows`), so the
  windows Locker impl uses `golang.org/x/sys/windows`. Amends D15's "vroot
  stays 2-dep" tally to three; the concern there was the sqlite/wazero tree,
  not near-stdlib x/sys. Rejected: hand-rolled
  `syscall.NewLazyDLL("kernel32.dll")` procs.
- **D26 — dependency note (informational).** ncruces/go-sqlite3 v0.35.3 no
  longer pulls wazero directly; the wasm payload comes via
  `github.com/ncruces/go-sqlite3-wasm/v3` (+ `ncruces/julianday`). D13's
  "ncruces+wazero" consequence reads accordingly.

## Final-review round (2026-08-14)

- **D27 — release ordering: vroot must land and tag before the new module's
  go.mod pin is bumped.** The final review verified `GOWORK=off go build`
  fails in vroot-adapter/overlayfs: its go.mod pins vroot
  v0.0.0-20260528191442-457556e7ce33, which predates `vroot.Locker` and the
  `osfs.File` wrapper — both part of this same change set, so no commit
  exists to pin to yet. Development builds work via the gitignored go.work
  (the repo's convention precisely to avoid `replace` directives). Required
  ordering at release: commit/tag vroot (Locker + osfs wrapper + x/sys dep)
  first, then `go get` the new pseudo-version in vroot-adapter/overlayfs and
  commit the pin bump. Until then CI for the new module cannot pass on a
  plain checkout.
- **D28 — DataSource gains exported copy-up accessors (user-confirmed).**
  The review found `CopyUpPolicy` was advertised as a public extension point
  while `*DataSource` exported only `Close` — an out-of-package policy could
  not reach the top's merged/ or work/ trees. Chosen: export `Fsys()`,
  `ContentPath(name)`, `StagingPath(name)` on `*DataSource`, matching what
  the default policy uses (additive; aligns with IDEA's "copy-up strategy
  stays extensible"). Rejected: deleting the public-extension claim for v1.
