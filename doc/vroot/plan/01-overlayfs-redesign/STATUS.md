# STATUS.md — overlayfs redesign implementation

**Last updated:** 2026-06-17 (orchestration start)
**Branch:** `feat-rewrite-vroot`
**Driver:** main context (orchestrator); coding offloaded to opus subagents.

## TL;DR

Implementing `doc/vroot/plan/01-overlayfs-redesign/PLAN.md` from scratch. The
old `vroot/overlayfs` was removed; nothing is coded yet. Design is fully settled
in PLAN.md. See `DECISION.md` for autonomous implementation-level decisions.

## Environment prep (done)

- `go.work` created at repo root (`use ./fsutil ./vroot`), gitignored. Wires
  local fsutil → vroot so `fsutil.TruncateUTF8` is visible. Workspace builds
  green. (DECISION D1)

## Orchestration plan (waves)

Each wave is verified **empirically by the orchestrator** (build + test +
sometimes probe tests) before the next starts. Coding done by opus subagents
(may spawn sonnet helpers).

- **Wave 1 (parallel):**
  - W1-A: `fsutil.TruncateUTF8` + test. *(module: fsutil)*
  - W1-B: overlayfs pkg skeleton + `MetadataStore` (iface), `MetadataStoreMem`,
    `MetadataStoreLog` (append+compaction+group-commit), `SubMetadataStore`,
    `LogOption`/`SyncMode`, contract test + bench. *(PLAN phase 1)*
- **Wave 2 (overlayfs foundation; serialized within package):**
  - `ovl_node.go` (ovlNode + ovlNodes: interning, per-node locks, singleflight,
    Load-seeding) + unit test. *(phase 2)*
  - `layer.go`/`layers.go` (Layer, NewLayer, merged lookups). *(phase 3)*
  - `copyuppolicy*.go` (interface + Stage + DotTmp). *(phase 4)*
- **Wave 3 (integration core):**
  - W3a: `resolve.go` + `file.go` + Fs struct/New/Option + READ methods +
    OpenRoot base-prefix. *(phase 5, 7-read)*
  - W3b: WRITE path (copy-up, Remove/RemoveAll whiteout, Mkdir opaque,
    Rename/Link/Symlink, Chmod/Chown/Chtimes, Sync/Close). *(phase 6)*
- **Wave 4:** tests — acceptance (`RunRoot`), overlay-specific
  (copyup/whiteout/opaque/symlink/listing), rename, concurrency `-race`,
  `fstest.TestFS`. *(phase 8)*
- **Wave 5:** README overlay section. *(phase 9)*

## Progress log

- [done] Read PLAN.md (design settled, all OQs resolved).
- [done] Mapped references; verified load-bearing facts directly.
- [done] Module wiring (go.work); workspace builds. DECISION.md/STATUS.md.
- [done] **Wave 1** — fsutil.TruncateUTF8 (+test); MetadataStore iface + Mem +
  Log (append/compaction/group-commit) + SubMetadataStore + contract test +
  bench. Verified: build, `-race`, bench all green; reviewed Log store source.
- [done] **Wave 2** — layer/layers (raw merged lower mechanics) + copyuppolicy
  (Stage + DotTmp). Verified build/`-race`; reviewed layers.go + copyuppolicy.go.
- [done] **Wave 3a** (subagent, died on API 529 but left working code) —
  ovl_node.go (table/locks/singleflight/GC), resolve.go (masking-aware lookup +
  symlink/confinement), file.go (merged dir handle + decShim), overlay.go read
  methods + OpenRoot. Verified: read-only acceptance + targeted masking tests
  pass under `-race`; reviewed ovl_node/resolve/overlay/file source.
- [done] **Wave 3b** (implemented BY MAIN CONTEXT — subagents kept dying on API
  529 overload; write path is the crux so main took it) — copy_on_write.go
  (ensureCopiedUp/ensureParentDirs) + all write methods in overlay.go
  (Chmod/Chown/Chtimes/Lchown/Create/OpenFile-write/Mkdir/MkdirAll/Remove/
  RemoveAll/Rename/Link/Symlink) + share_violation_*.go. Concurrency per D12
  (per-node lock + ordering, no self-deadlock). Tests: overlay_test.go
  (full RunRoot+RunFs acceptance), copyup_test.go (copy-up/whiteout/opaque/
  recreate/cross-layer/rename). **Verified empirically:** full acceptance
  (incl. all write subtests + Escapes + Race), overlay-specific semantics,
  `-race` clean (4.6s), vet clean, golangci-lint 0 issues. Found+fixed 2 real
  bugs via acceptance (parent-must-exist; RemoveAll(".")→EINVAL).
- [done] **Wave 4** (subagent) — concurrency_test.go (independent-path stress;
  copy-up singleflight dedup proven exactly-once via a counting policy with a
  widened race window; crossing-rename deadlock timeout; DisableOpenFileRemoval
  → EINVAL then succeeds-after-Close), fstest_test.go (`fstest.TestFS` over a
  merged+whiteout view), stage_visibility_test.go (visible/shared work dir),
  openroot_overlay_test.go (shared-node masking + confinement). Verified:
  inspected the singleflight + DisableOpenFileRemoval tests are genuine; full
  `-race` green; lint 0 issues.
- [done] **Wave 5** — `vroot/README.md` overlay section rewritten to the generic
  API (`New`/`NewLayer`/`MetadataStoreLog`/copy-up policies) + ovl-node /
  concurrency / whiteout-vs-opaque notes. Fixed a type bug in the example
  (`NewMetadataStoreLog` needs `vroot.Widen(top)`).
- [done] **Cross-platform fix** (main context) — added `share_violation_windows.go`
  and swapped `syscall.EROFS`→`errdef.EROFS` in file.go so overlayfs builds on
  windows/plan9/darwin/linux (sibling pkgs target all four). Verified via
  `GOOS=… go build`.

## FINAL STATE: COMPLETE ✅

All 9 PLAN phases implemented and empirically verified by the orchestrator:
- `go test -race ./overlayfs/...` → green (60 passing tests, 4.6s).
- `go vet` + `golangci-lint run ./overlayfs/` → 0 issues.
- `GOOS=linux|windows|plan9|darwin go build ./overlayfs/...` → all pass.
- Whole `vroot` module `go test ./...` → green (no regression); `fsutil` green.
- Full `acceptancetest.RunRoot`/`RunFs` pass (read+write+escapes+race); plus
  overlay-specific copy-up / whiteout / opaque-on-recreate (the old bug) /
  cross-layer symlink / rename / singleflight / fstest.TestFS / sub-overlay.

Notes:
- `go.work` (gitignored) wires local fsutil→vroot; for a committed release the
  human must publish fsutil (TruncateUTF8) and bump `vroot/go.mod`'s fsutil
  require (see DECISION D1). No commit/push performed (per session policy).
- Wave 3b (write path) was implemented by the MAIN CONTEXT, not a subagent,
  because the Anthropic API was returning 529 Overloaded and killing subagents
  mid-task; the write path is the correctness crux so main took it and verified
  empirically. Waves 1/2/3a/4 were done by opus subagents (3a's subagent also
  died on 529 but had already written working code, confirmed by tests).
- Concurrency model: see DECISION D12 (per-node lock + careful ordering; not
  literal held cross-backend transactions, to avoid RWMutex self-deadlock with
  the ancestor masking walk).

## Key references (for any agent picking this up)

- Primary impl reference: `vroot/synthfs/` (Root-only, shared-state OpenRoot via
  `boundary` + `name`, `fsutil.WrapPathErr`/`WrapLinkErr`, refCount handle
  tracking, dir cursor handle).
- Old impl (ideas only): `git show 15c85d6:vroot/overlayfs/<file>`.
- Helpers: `fsutil.ResolvePath(fsys{ReadLinkFs,LstatFs}, name, skipLast)`,
  `fsutil.OpenFileRandom`, `fsutil.WrapPathErr/WrapLinkErr`,
  `vroot.Widen[F]`, `vroot/internal/openflag.WriteOp`,
  `vroot/internal/paths.PathFromHead`, `clock.WallClock`.
- Acceptance wiring example: `vroot/synthfs/root_test.go`
  (`acceptancetest.RunRoot` / `RunFs`).

## Open risks / things to scrutinize empirically

- Cross-layer atomicity invariant (ovl-node lock spanning top-write + journal +
  lower-lookup) — verify under `-race`.
- Whiteout subtree-masking vs opaque-dir distinction (the old recreation bug).
- Merged dir listing filtering (whiteout/opaque) — old bug.
- copy-up singleflight dedup (exactly one copy-up under concurrent writers).
- OpenRoot base-prefix confinement (`..`/symlink escapes) — acceptance Escapes.
