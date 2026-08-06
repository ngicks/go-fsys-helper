# DECISION.md — autonomous implementation decisions

This file records decisions made *without* the user, that would otherwise have
required an interview. The high-level design is already settled in
`doc/vroot/plan/01-overlayfs-redesign/PLAN.md` (§2 decision log, all OQs
resolved). The items below are **implementation-level** calls that the PLAN
left open or that arose while building.

Format: **Decision → Rationale → Alternatives weighed → Reversibility.**

---

## D1. Local module wiring: gitignored `go.work` (not `replace`)

- **Decision:** Added a repo-root `go.work` (`use ./fsutil ./vroot`). It is
  **gitignored** (`.gitignore` already lists `go.work`/`go.work.sum`), so it is
  a local-dev artifact, not a committed change.
- **Rationale:** `vroot/go.mod` consumes `fsutil` as a *pinned pseudo-version*
  (`v0.0.0-...457556e7ce33`, matching local commit `457556e`). There is no
  `replace` and no committed `go.work`. The plan adds `fsutil.TruncateUTF8`
  (new symbol in fsutil) that `vroot/overlayfs` must call. Without wiring, the
  local fsutil change is invisible to vroot. `.gitignore` listing `go.work`
  proves the maintainer's local-dev convention *is* a workspace file.
- **Alternatives weighed:**
  - `replace` directive in `vroot/go.mod` — pollutes a committed file with a
    local path; contrary to the repo's pseudo-version workflow.
  - Bump `vroot/go.mod`'s fsutil require to a new pseudo-version — requires the
    fsutil change to be committed *and* fetchable via GOPROXY/VCS; brittle in a
    possibly-restricted env.
  - Put `TruncateUTF8` inside vroot instead of fsutil — contradicts the PLAN
    (§4, §7 explicitly say `fsutil.TruncateUTF8`, `fsutil/truncate_utf8.go`).
- **Reversibility:** High. Deleting `go.work` restores the original resolution.
- **Follow-up for the human:** When publishing, commit the fsutil change and
  bump `vroot/go.mod`'s fsutil require to the new pseudo-version (out of scope
  here — no push/publish performed).

## D2. `ovlNodes` table concurrency: single `sync.RWMutex`, not sharded (v1)

- **Decision:** The interning table uses one `sync.RWMutex` guarding the map
  structure only (never held across I/O). No sharding in v1.
- **Rationale:** PLAN §5.0 explicitly marks sharding optional ("shardable if it
  shows up in profiles"). Correctness first; the table lock is only held for
  map insert/lookup/delete, so it serializes bookkeeping, not operations.
- **Alternatives weighed:** Sharded `[]shard{mu;byPath}` keyed by path hash —
  more code, premature without a profile.
- **Reversibility:** High; sharding is an internal change behind the same API.

## D3. Default copy-up policy = `CopyUpPolicyStage`; acceptance `Make` uses `DotTmp`

- **Decision:** `New`/`DefaultOption` default to `CopyUpPolicyStage("")` (visible
  work dir). The generic acceptance-test `Make` wires `CopyUpPolicyDotTmp` so the
  standing work dir does not appear in strict listing assertions.
- **Rationale:** PLAN decision 21 (latest changelog) sets Stage as default; §8
  test plan note says acceptance `Make` uses DotTmp, with a dedicated test for
  the Stage work dir's visibility/sweep. Both are honored.
- **Reversibility:** High; both policies ship.

## D4. Acceptance-test fixture: memfs top + two empty memfs lowers

- **Decision:** The overlay acceptance `Make(t, lines)` builds
  `top = memfs.New(...)`, two empty `memfs.New(...)` lowers, `MetadataStoreMem`,
  `CopyUpPolicyDotTmp`, then materializes `lines` through the overlay's own
  write methods. Acceptance `Option` mirrors synthfs's (symlinks on, Chown
  stored uid/gid=1000) since memfs == synthfs defaults.
- **Rationale:** memfs is synthfs with defaults; it supports
  Seek/ReadAt/WriteAt/symlink and stores Chown — no skip flags needed (matches
  `synthfsOption()`). Ephemeral store keeps the suite hermetic and fast.
- **Alternatives weighed:** osfs tmp-dir top — exercises real disk + real
  `Rename` atomicity, but adds platform variance to the generic suite. Kept for
  a *targeted* test instead of the generic acceptance run.
- **Reversibility:** High.

## D5. Journal defaults: `Sync = Batched`, background writer via `errgroup`

- **Decision:** `MetadataStoreLog` defaults to `Batched` group commit with an
  `errgroup`-managed background writer (bounded channel; fsync per batch /
  debounce). `Flush()` forces pending records durable; `Close` flushes.
  `LogOption` zero-values: `CompactFactor→2`, `CompactMin→64`, `Sync→Batched`.
- **Rationale:** PLAN decisions 24/23, §4, §6. Keeps fsync off the hot path.
- **Reversibility:** Medium; knob is public (`SyncMode` PerOp|Batched|None).

## D6. Constant/name choices

- `DefaultWorkDir = ".vroot-overlayfs.work"` (PLAN §4).
- DotTmp temp name: `TruncateUTF8(base, nameMax-22) + "." + rand10 + ".copyup.tmp"`
  where 22 = 10 (rand) + 1 (dot) + 11 (`.copyup.tmp`); `nameMax` default 255.
- `ErrTypeNotSupported = errors.New("type not supported")` (PLAN §4).

## D9. Lower-layer `meta` is accepted but not consulted (v1)

- **Decision:** `NewLayer[F](fsys, meta)` stores `meta` on every Layer, but the
  overlay uses ONLY the **top** layer's `meta` as its journal (seeds the
  ovl-node table via `top.meta.Load()`). Lower layers' `meta` is accepted for
  API symmetry and ignored. `meta` may be nil (lowers, or ephemeral overlays).
- **Rationale:** PLAN decision 12 (lowers immutable) + the ovl-node table being
  authoritative and seeded from a single store (`top.meta`). A plain read-only
  lower carries no overlay deviation-state of its own. PLAN §4 keeps the
  symmetric `NewLayer(fsys, meta)` signature, so the field stays for
  forward-compat (e.g. a frozen lower exported from a prior overlay session).
- **Reversibility:** High; the field already exists.

## D10. `New` cannot return an error → Load failure starts an empty table

- **Decision:** `New(top, lowers, opt) *Fs` has no error return (PLAN §4). If
  the seed `store.Load()` fails inside `newOvlNodes`, `New` keeps the
  (empty-but-valid) table over the same store and returns a usable `*Fs`;
  persisted whiteouts/opaques are simply NOT seeded that run.
- **Rationale:** A corrupt/unreadable journal must not make construction panic.
  Starting empty is a safe degraded mode: content reads are unaffected; only
  prior *deletions* (whiteouts) and opaque markers are not re-applied. Callers
  who need to detect this can call `(*Fs).Sync()`/inspect the store themselves.
- **Alternatives weighed:** panic (too harsh for an I/O error); change the
  signature to return error (deviates from the settled PLAN API).
- **Reversibility:** High.

## D11. Close semantics: sub-overlay no-op, root closes shared resources

- **Decision:** A sub-overlay from `OpenRoot` (`sub==true`) shares the ovl-node
  table, top, lowers, store and policy; its `Close()` is a no-op. The root
  overlay's `Close()` flushes the store, closes it if it is an `io.Closer`
  (the Log store), and closes `top` + every lower, joining errors.
- **Rationale:** Closing shared filesystems from a sub-overlay would break the
  parent and siblings. Mirrors how synthfs sub-roots don't tear down shared
  state. PLAN decision 10 ("Close top + every lower, gather errors") applies to
  the owning root.
- **Reversibility:** High.

## D12. Write-path concurrency: per-node lock + careful ordering (not held cross-backend transactions)

- **Context:** PLAN §5.0 frames the per-node `sync.RWMutex` as a held
  transaction boundary spanning top-write + journal-flip + lower-lookup. A
  *literal* held lock self-deadlocks: a writer holding node[full]'s write lock
  that then calls `lookup(full)`/`ancestorWhiteout(full)` would re-RLock the
  same node (Go RWMutex is non-reentrant).
- **Decision (v1):** Achieve the same observable guarantee — *no resurrection,
  no torn read* — with two mechanisms:
  1. **Per-node write lock for structural single-path ops** (Mkdir/Remove/
     RemoveAll/Symlink/create-open) to serialize same-path writers. Inside that
     lock the writer uses ONLY: direct reads/writes of the held node's bits,
     **strict-ancestor** masking checks (`ancestorWhiteout(path.Dir(full))`,
     `ancestorOpaque(full)` — both lock *other* nodes only), and raw
     `top`/`lower` ops. It never calls `lookup(full)` or the re-locking helper
     mutators on `full`.
  2. **Operation ordering** so every intermediate state a lock-free reader can
     observe is consistent: on delete, set the whiteout bit+journal BEFORE
     `top.Remove` (a reader sees either "present, no whiteout" or "whiteout →
     ENOENT", never a resurrected lower); on recreate-over-whiteout, `top.Mkdir`
     → `setOpaque` → `clearWhiteout` last (full only becomes visible once both
     top-dir and opaque are in place).
  - Copy-up always runs via `nodes.copyUp` (singleflight) with NO node lock held
    (it locks the node internally to flip `up`). Metadata ops (Chmod/Chown/
    Chtimes/Lchown) and write-open of an existing file copy-up first, then do the
    top op without a node lock (they don't change masking; top's op is atomic).
  - Cross-path ops (Rename/Link) lock the two involved nodes in **canonical
    lexical order** of full paths to avoid deadlock.
- **What this guarantees:** data-race freedom (all node/table access via the
  synchronized ovl_node methods or a held node lock; verified under `-race`);
  independent-path parallelism; copy-up dedup; the no-resurrection invariant.
- **What it does NOT guarantee:** strict linearizability of two *concurrent
  mutations of the same path* beyond what per-node serialization + ordering
  give. PLAN's stated concurrency goal (independent-path parallelism + copy-up
  dedup + no resurrection) is met. Strengthening to full held-lock compound
  transactions is a future refinement.
- **Reversibility:** Medium; internal to the write path.

## D7. overlayfs is a package within the vroot module

- **Decision:** No separate `go.mod`; `vroot/overlayfs` is a package in the
  `github.com/ngicks/go-fsys-helper/vroot` module (matches synthfs/memfs/osfs).

## D8. Build/lint hooks

- **Observation:** PostToolUse hooks run `go vet -vettool=ngcheckers` and
  `golangci-lint` on the *directory* of each written file. Writing non-Go files
  (e.g. `go.work`, `.md`) at dirs with no Go files yields a benign "no Go files"
  hook error — ignored. For real `.go` files the linters are authoritative and
  must pass; subagents are instructed to satisfy them.
