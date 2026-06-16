# overlayfs redesign — PLAN

- **Status:** Design settled (2026-06-16) — all open questions resolved; ready to implement. Nothing coded yet.
- **Target module:** `github.com/ngicks/go-fsys-helper/vroot/overlayfs` (+ a small enabling addition to `vroot` core)
- **Branch:** `feat-rewrite-vroot`
- **Last updated:** 2026-06-16

## How to use this doc

Same workflow as `synthfs-redesign/PLAN.md`. This is a living design doc. §2 records what I'd settle and why (each is a *recommendation*, not yet locked); §3 lists what genuinely needs your call before coding. Answer §3 here or in chat and I'll fold the result back in. Implementation starts once §3 is resolved.

Scope of this doc, per request: **basic logic, data structure, public API, persistent data shape.** Implementation phasing (§9) is sketched but secondary.

---

## 1. Background

### 1.1 What overlayfs is (and was)

`overlayfs` is a **union mount**: a writable **top** layer stacked over one or more read-only **lower** layers. Reads resolve top-down (top wins); writes always land in top; files that live only in a lower layer are **copied up** to top before being modified; deletions are recorded as **whiteouts** rather than mutating the read-only lowers.

> **Design stance (2026-06-16):** we are *not* porting the old implementation. We reuse its *ideas* (top/lower stacking, copy-up, whiteout) but rebuild the internals around a **sparse in-memory node graph** that owns the overlay's deviation-state and the per-path locks (§5.0). This replaces the old single global `sync.RWMutex` with fine-grained, per-path concurrency.

The previous implementation (removed in `b1d69b8`, recoverable at `15c85d6:vroot/overlayfs/`) was complete and tested but targeted the **pre-generics** interfaces (`vroot.Rooted` / `vroot.Unrooted`) and serialized everything behind one lock. It is a *reference*, not a base. Its shape:

```
Fs            { rw *sync.RWMutex; opts; top vroot.Rooted; topMeta MetadataStore; layers Layers }
Layer         { meta MetadataStore; fsys vroot.Rooted }   // read-only + whiteout filter
Layers        []Layer                                     // merged lowers, highest-priority last
MetadataStore { RecordWhiteout / RemoveWhiteout / QueryWhiteout }
CopyPolicy    { CopyTo(from Layer, to vroot.Rooted, name) }   // default: copy to *.tmp then rename
overlayFile   { read-only merged view; writes go straight to top after copy-up }
layersFile    { concatenated dir handles across lowers, dirent cache }
```

Persistence today: top content lives in whatever `vroot.Rooted` is the top (e.g. osfs on disk); whiteouts live in a side-band `MetadataStore`, default `MetadataStoreSimpleText` — a newline-delimited file of `strconv.Quote`d slash paths, held in memory as a path-tree, rewritten atomically (`whiteout.tmp` → `whiteout`) on every change.

### 1.2 The generic core it must target now

```go
type Fs[F File] interface { Chmod(...); Open(name) (F, error); OpenFile(...); ... }
type Root[F File, R Root[F, R]] interface { Fs[F]; IsRoot(); OpenRoot(name) (R, error) }
type File interface { /* ~*os.File minus syscall-specific bits */ }
```

Conventions established by `osfs` / `synthfs` (the references):
- A pure virtual fs is **Root-only**; `*Root` implements `vroot.Root[vroot.File, *Root]` and gets the full `Fs` suite for free via embedding.
- Heterogeneous file types are erased to `vroot.File`; `vroot.Widen[F](Fs[F]) Fs[File]` bridges `Fs[*os.File]` → `Fs[File]`.
- Read-only wrappers: `vroot.NewReadOnlyFs[F]`, `vroot.NewReadOnlyRoot[F,R]`.
- Acceptance: `acceptancetest.RunRoot(t, SetupRoot[vroot.File, *overlayfs.Fs]{Make})` where `Make(t, lines)` returns a fresh, pre-populated fs.
- Helpers available: `fsutil.ResolvePath` (symlink resolution over an `Lstat`/`ReadLink` pair, with `..`/escape rejection), `fsutil.WrapPathErr` / `WrapLinkErr`, `fsutil.OpenFileRandom`, `vroot/internal/paths.PathFromHead`, `vroot/internal/openflag.WriteOp`, `clock.WallClock`.

### 1.3 Why the rewrite is not mechanical

1. **Layer type erasure.** Lowers are heterogeneous (`osfs.Root` with `*os.File`, `memfs`/`synthfs.Root` with `vroot.File`, `sftpfs`, …). The old code stored them behind the non-generic `vroot.Rooted` interface. Under generics the drop-in equivalent already exists: store layers as **`vroot.Fs[vroot.File]`**, erased via the existing **`vroot.Widen`**. We do **not** need a type-erased *Root* (`RootFile`): the overlay supplies its own `OpenRoot` (base-prefix, §5.5), so layers never need `OpenRoot`, and `Fs[File]` already has every method the overlay calls on a layer (decision 2).
2. **Whiteout vs opaque conflation.** The old `QueryWhiteout` made a whiteout on a directory mask the whole subtree, and *nothing* cleared it on recreation — re-`Mkdir`ing a deleted directory and adding a child made the child read back as deleted (parent still whited). Real overlay semantics need a separate **opaque-directory** concept. The rewrite should fix this (decision 5).
3. **Directory merge dropped whiteouts.** `layersFile.readDir` merged lower dirents without filtering whiteouts/opaque, so deleted entries could reappear in listings. Must be corrected (decision 6).
4. **Cross-layer atomicity (the real driver).** A single logical overlay operation spans **independent backends that share no lock**: it reads from a lower (`osfs`, `sftpfs`, …), writes to top (`memfs`, `osfs`, …), and flips whiteout/opaque in a side-band journal. *No backing filesystem can make that compound op atomic* — `osfs` locks don't cover `memfs`, and neither covers the journal. So the overlay itself must be the coordination point. The old `Fs` did this with one global `sync.RWMutex` (it even carried `// TODO: use finer lock mechanism`); the rewrite moves the coordination into a per-path node graph (decisions 9, 11, 12; §5.0). Fine-grained concurrency is the *benefit*; **correct cross-layer atomicity is the requirement.**

---

## 2. Decision log (recommended; not yet locked)

| # | Decision | Choice | Rationale |
|---|----------|--------|-----------|
| 1 | Interface & file type | `*overlayfs.Fs` implements `vroot.Root[vroot.File, *Fs]`; **Root-only** | A union over rooted layers is inherently rooted. `vroot.File` erases heterogeneous layer files. `*Fs` satisfies `vroot.Fs[vroot.File]` via embedding, so `RunRoot` exercises the whole `Fs` suite + `OpenRoot` + escapes. Matches synthfs. |
| 2 | Layer storage / erasure | **No new core type.** Store layers as the existing **`vroot.Fs[vroot.File]`**, erased via the existing **`vroot.Widen`**; overlay provides its own `OpenRoot` (base-prefix over a shared graph), so layers need neither `OpenRoot` nor `IsRoot` (OQ2 — revised) | Avoids inventing `RootFile`/`WidenRoot` and the `Widen`→`WidenFs` rename. `Fs[File]` already carries every method the overlay calls on a layer (reads + writes). Confinement is preserved when callers pass rooted impls — a widened `osfs.Root` still confines, since erasure changes only the file type, not behavior — and the overlay enforces its own boundary regardless (decision 3). **Zero vroot core change.** |
| 3 | Layer confinement | Layers typed `vroot.Fs[vroot.File]`; **pass rooted impls** for defense-in-depth; the overlay enforces its own boundary (`..` + cross-layer symlink resolution) against each (sub-)root's base | `Fs` can't assert rootedness at the type level, but a widened `osfs.Root`/`memfs`/`synthfs.Root` still confines at runtime. The overlay must resolve symlinks across the merged view *anyway*, so it owns confinement; rooted layers are a backstop, not a requirement. |
| 4 | Constructor shape | `New(top Layer, lowers []Layer, opt *Option) *Fs`; `NewLayer[F vroot.File](fsys vroot.Fs[F], meta MetadataStore) Layer` | `NewLayer` widens `F→File` via `vroot.Widen` at the boundary (no-op if already `Fs[File]`). `lowers` ordered low→high priority; resolution `top → lowers[n-1] → … → lowers[0]` (carried from old `slices.Backward`). |
| 5 | Deletion model | **Whiteout (subtree-masking) + Opaque (dir-local)**, two distinct records | Whiteout(`p`): `p` and everything that would be under it in lowers is hidden. Opaque(`d`): `d` exists in top but lower children of `d` are hidden while top children show. Creating an entry at a whited path **clears** that whiteout; recreating a directory over a whiteout sets it **opaque**. Fixes the recreation bug. |
| 6 | Directory listing | Merge top+lowers, dedup by name (top wins), **drop whited names, stop at opaque** | Correctness fix over old `layersFile.readDir`. Lower dirents are filtered through `MetadataStore` before being yielded. |
| 7 | Copy-up | Retain `CopyPolicy`; default `CopyPolicyDotTmp` (copy → `*.tmp` → rename, best-effort metadata) | Unchanged design; copy-up of a parent dir for a child write is **not** opaque (lowers stay visible). Triggered by every mutating op (write-open, Chmod/Chown/Chtimes, Rename, Link, Symlink-over). |
| 8 | Persistent metadata shape | Pluggable `MetadataStore` used as a **load-once + write-behind journal** feeding the node graph; default = **append-log** store (decision 23), human-readable, side-band | The graph (decision 12) is the in-memory truth; the store is loaded at construction and written-behind on change, off the hot path. See §6. In-band AUFS-style markers are the alternative (OQ4). |
| 9 | Concurrency model | **Sparse in-memory node graph** with **per-node `sync.RWMutex`** as the **cross-layer transaction boundary**; no single global lock | A node per *touched* path holds the overlay's deviation-state (whiteout/opaque/copy-up/open-handles) and is the lock unit. The node lock is the *only* thing that makes a top-write + journal-flip + lower-read commit atomically (the backends share no lock — §1.3.4). Independent paths never contend; untouched reads bypass the graph and hit the layers directly (read-safe). §5.0. |
| 10 | `Close` | Close top + every lower, gather errors (`serr`-style) | Idea carried from old. |
| 11 | Copy-up dedup | `golang.org/x/sync/singleflight` keyed by path | Per-repo concurrency rule. Concurrent writers to the same lower-only file collapse to one copy-up; no lock held across copy IO; writers to different paths never serialize. |
| 12 | State authority & ownership | Node graph is **authoritative** in-memory state; **lowers are immutable** (confirmed), overlay is **sole writer of top** | Lower immutability makes the under-lock lower-lookup race-free, so the node lock is a sufficient cross-layer transaction boundary; it also lets the graph cache lower resolution/dirents node-wide (they never change). Sole-writer-of-top is the standard single-writer rule; top existence is still revalidated per op, so the `up` flag is a self-healing hint, not a trusted cache. (OQ9 resolved.) |
| 13 | Recreate-dir semantics | Delete-then-recreate of a lower-backed dir ⇒ **opaque** (lower children stay hidden) (OQ3) | Matches `rm -rf dir && mkdir dir` intuition and Linux overlayfs. `Mkdir` over a cleared whiteout that had lower content sets the node's `opaque` bit. |
| 14 | `MetadataStore` shape | **Journal**: `Load()` + four single-change persisters; no query methods (OQ6) | Graph is truth (decision 9/12); the store only seeds at construction and durably records changes. Minimal surface. §5.2. |
| 15 | Graph materialization | **Lazy** — node created only when a path is touched (OQ7) | Memory ∝ touched paths; resolution checks ancestor nodes hand-over-hand. Fits the sparse, no-staging design. |
| 16 | Node GC | **GC pure-cache nodes** — evict when no whiteout/opaque, copy-up settled, zero open handles (OQ8) | Copy-up state is re-derivable from `top.Lstat`; whiteout/opaque/open nodes are always retained. Bounds memory. |
| 17 | Top requirement | **Require a writable top** in v1 (OQ5) | Single code path; a read-only merge is `vroot.NewReadOnlyRoot` over the result later. |
| 18 | Metadata locality | **Side-band** default store; in-band markers demoted to a "possible later" note (OQ4) | Safe because lowers are read-only — side-band can't diverge from lower content; keeps top namespace clean. §6. |
| 19 | Work area ("workdir") | An **explicit, VISIBLE** dir at the top root (default `.vroot-overlayfs.work`), **owned by `CopyUpPolicyStage`**; **no hiding, no magic**; lazy-created + self-swept by the policy; shared by all sub-overlays | Atomic publish needs temp+dest on one fs; the single top `Fs` gives that — `top.Rename(workdir/tmp, dest)` is one call → atomic. Per review (decision 26): the overlay does **not** hide it — the work dir is apparent to the caller, exactly like Linux overlay's caller-designated `workdir`. The caller picks a non-colliding name and knows it appears in the merged view. §5.6. |
| 20 | Policy rename | `CopyPolicy`→**`CopyUpPolicy`**, `CopyTo`→**`CopyUp`** | Direction-correct: copy-up is specifically lower→top. |
| 21 | Default policy | **`CopyUpPolicyDotTmp`** (in-place temp beside dest → atomic `Rename`); `CopyUpPolicyStage` is an explicit opt-in | Flipped from Stage once the work dir became visible (decision 26): DotTmp leaves **no standing artifact** (temps are transient, renamed away), so a default overlay's listing stays clean. Pick `CopyUpPolicyStage` when you want scratch corralled in one (visible) work dir and accept it in the tree. *(Reversible — see §3 note.)* |
| 22 | `.copyup.tmp` policy | In-place temp `"<truncated-base>.<rand>.copyup.tmp"`; UTF-8-aware truncation via new `fsutil.TruncateUTF8` to fit the policy's `nameMax` (ctor arg, default 255) | Distinctive suffix avoids collision with user `.tmp` and eases cleanup; base-derived name aids identifiability; rune-boundary truncation handles ~255-byte source names; random restores uniqueness lost to truncation. §5.6. |
| 26 | Staging config ownership; no hiding | The `CopyUpPolicy` **owns** its staging detail (work dir / `nameMax`) as ctor args, not `Option`. **`ReservedNamer` removed** — the overlay hides nothing; a work dir is an explicit, visible directory the caller designates | A policy-specific knob doesn't belong on `Option`. Implicit name-hiding was hard to communicate and magic; making the work dir apparent (Linux-`workdir` style) is clearer and removes overlay↔policy coupling entirely. `CopyUp` takes loose `(from, to, name)` args. |
| 23 | Default durable store | **`MetadataStoreLog`** — append-only journal + threshold compaction; plus **`MetadataStoreMem`** (no-op) for ephemeral overlays; keep `SimpleText` as a minimal variant | `SimpleText`'s full-rewrite-per-change is O(N)/op → O(N²) for N deletes. Append+compaction is O(1) amortized; `Load`=replay; fits the journal interface (decision 14) unchanged. `Mem` is zero-cost when durability isn't needed. §6. |
| 24 | Persist off hot path | Optional **single background writer + group commit** (`fsync` per batch/debounce); durability is a knob (per-op ↔ batched ↔ none) | Removes `fsync` (the real cost) from the calling op; serializes only journal I/O, not filesystem ops. `errgroup` lifecycle; flush on `Close`. |
| 25 | Compaction normalization | At compaction, drop whiteout/opaque records subsumed by an ancestor dir-whiteout/opaque | Keeps log + live set minimal; free since the graph answers queries regardless. |

### `vroot` core: no change needed (decision 2 — revised)

Layers are stored as the existing `vroot.Fs[vroot.File]` and erased with the existing `vroot.Widen[F](Fs[F]) Fs[File]`. No `RootFile`/`WidenRoot` is introduced, and `vroot.Widen` is **not** renamed — the earlier `WidenFs` rename existed only to pair with `WidenRoot`, which is now dropped. The overlay supplies its own `OpenRoot` (base-prefix over a shared graph, §5.5), so a layer never needs to expose `OpenRoot`/`IsRoot`. A widened `osfs.Root` keeps confining symlinks at runtime (erasure changes only the file type), so confinement survives the erasure for rooted inputs.

---

## 3. Decisions (all open questions resolved 2026-06-16)

Every OQ is now answered (via review + the AskUserQuestion round). Captured as decisions in §2; recorded here for traceability.

| OQ | Question | Resolution | Decision |
|----|----------|------------|----------|
| OQ1 | Plan file location | Directory + `PLAN.md` (mirrors `synthfs-redesign/`) | — |
| OQ2 | Home for `RootFile`/`WidenRoot` | **Revised — not needed.** Store layers as existing `vroot.Fs[vroot.File]` (erased via existing `vroot.Widen`); no new core type, no `Widen` rename | 2, 3 |
| OQ3 | Recreate-dir semantics | **Opaque** — lower children stay hidden after delete-then-recreate | 5, 13 |
| OQ4 | Metadata side-band vs in-band | **Side-band** default; in-band demoted to "possible later" | 8, 18 |
| OQ5 | Require a writable top? | **Yes**, require a top in v1 | 17 |
| OQ6 | `MetadataStore` shape | **Journal** — `Load()` + four persisters, no query methods | 14 |
| OQ7 | Graph lazy vs eager | **Lazy** | 15 |
| OQ8 | Node GC | **GC pure-cache nodes** (retain whiteout/opaque/open) | 16 |
| OQ9 | Backend ownership | **Lowers immutable** (confirmed); overlay **sole writer of top** | 12 |

### Locked defaults (no further confirm needed)

- **Resolution order** = `top → lowers[n-1] → … → lowers[0]` (last lower = highest priority), matching the old README diagram.
- **Default `CopyPolicy`** = `CopyPolicyDotTmp("*.tmp")`, best-effort mode/mtime, no ownership/xattr copy.
- **Lock granularity** = per-node `sync.RWMutex` + short bookkeeping lock on the interning map (shardable) + `singleflight` for copy-up; `Rename`/`Link` lock the two parent-dir nodes in canonical (lexical) order (decisions 9, 11).

> **One thing to confirm (default policy flip, decision 21).** Removing the implicit hiding (decision 26) made the `Stage` policy's work dir *visible*, which is a poor default, so I flipped the default to `CopyUpPolicyDotTmp` (no standing artifact). If you'd rather keep `CopyUpPolicyStage` as the default and accept a visible `.vroot-overlayfs.work` in every overlay's root, say so and I'll revert.

**Status: design settled — ready to implement (§9 phases).**

---

## 4. Public API (consolidated)

```go
package overlayfs // github.com/ngicks/go-fsys-helper/vroot/overlayfs

// ---- Fs ----

type Fs struct{ /* g *graph (shared across sub-roots), top, lowers, base-prefix, opts */ }

var _ vroot.Root[vroot.File, *Fs] = (*Fs)(nil)

// New builds a union mount. top is writable; lowers are read-only, ordered
// low→high priority. opt nil ⇒ DefaultOption. New calls top.meta.Load once to
// seed the node graph.
func New(top Layer, lowers []Layer, opt *Option) *Fs

type Option struct {
    // CopyUpPolicy selects the copy-up strategy and OWNS its own staging detail
    // (work dir / NAME_MAX live on the policy, not here). nil →
    // NewCopyUpPolicyDotTmp(0) (the default — leaves no standing artifact).
    CopyUpPolicy CopyUpPolicy
    // DisableOpenFileRemoval rejects Remove/Rename of a path with live open
    // handles (windows-like ERROR_SHARING_VIOLATION), using node handle counts.
    DisableOpenFileRemoval bool
    // (future: Clock, MaskChmodMode — mirror synthfs Option if needed)
}
func DefaultOption() *Option

// *Fs implements every vroot.Root[vroot.File, *Fs] method. OpenRoot returns a
// *Fs sharing the same graph/top/lowers, carrying a base path prefix;
// confinement to the sub-path is enforced by the overlay's own boundary check
// (not a layer OpenRoot). Metadata via SubMetadataStore(base).

// ---- Layer ----

type Layer struct{ /* meta MetadataStore; fsys vroot.Fs[vroot.File] */ }

// NewLayer erases the layer's concrete file type via vroot.Widen (no-op if the
// layer is already Fs[vroot.File]). Pass a rooted impl (osfs.Root, memfs,
// synthfs.Root) so confinement is preserved at runtime; the overlay enforces
// its own boundary regardless.
func NewLayer[F vroot.File](fsys vroot.Fs[F], meta MetadataStore) Layer

// ---- MetadataStore (journal; see §5.2, §6) ----

type MetadataStore interface { /* Load() + Set/Clear Whiteout/Opaque */ }

func NewMetadataStoreMem() *MetadataStoreMem                          // ephemeral, no-op persistence
func NewMetadataStoreLog(fsys vroot.Fs[vroot.File], opt *LogOption) *MetadataStoreLog // DEFAULT durable: append + compaction
func NewMetadataStoreSimpleText(fsys vroot.Fs[vroot.File]) *MetadataStoreSimpleText   // minimal full-rewrite variant
func SubMetadataStore(s MetadataStore, base string) MetadataStore

type LogOption struct {
    CompactFactor int  // compact when appended > live*factor (0 → 2)
    CompactMin    int  // …and appended > this floor (0 → 64)
    Sync          SyncMode // PerOp | Batched | None (0 → PerOp)
}

// ---- CopyUpPolicy (renamed from CopyPolicy; direction-correct) ----

type CopyUpPolicy interface {
    // CopyUp copies the file/dir/symlink at name from a lower layer (from) up to
    // the top (to), publishing it at name (a full top path). The policy OWNS its
    // staging detail (where temps live, NAME_MAX) and can derive any extra info
    // (e.g. FileInfo) via from.Lstat(name); the overlay passes only the layers
    // and the path. Mirrors the old CopyPolicy.CopyTo's three-arg shape.
    CopyUp(from, to vroot.Fs[vroot.File], name string) error
}

// Built-ins — each owns its staging detail, lazy-creates its workspace and
// self-sweeps leftover *.copyup.tmp. No overlay magic: nothing is hidden.
func NewCopyUpPolicyDotTmp(nameMax int) *CopyUpPolicyDotTmp    // DEFAULT; in-place "<trunc-base>.<rand>.copyup.tmp" beside dest, then Rename; 0 → 255. No standing dir.
func NewCopyUpPolicyStage(workDir string) *CopyUpPolicyStage   // explicit, VISIBLE work dir at the top root ("" → DefaultWorkDir); temp-in-workdir → atomic Rename.
const DefaultWorkDir = ".vroot-overlayfs.work"
var ErrTypeNotSupported = errors.New("type not supported")
```

`from`/`to` are plain `vroot.Fs[vroot.File]` so policies don't depend on the whiteout machinery — they only copy bytes/metadata.

### New `fsutil` helper (for temp-name building)

```go
// TruncateUTF8 returns the longest prefix of s that is valid UTF-8 and at most
// maxBytes bytes — never splitting a multi-byte rune. Used to fit
// "<base>.<rand>.copyup.tmp" within a filesystem's NAME_MAX.
func TruncateUTF8(s string, maxBytes int) string
```

---

## 5. Data structures & basic logic

### 5.0 The node graph (concurrency core)

The overlay keeps a **sparse, lazily-built in-memory graph** of the paths it has *deviated* from the raw stacked layers. It is **not** a content store (synthfs is that) and **not** a full mirror of the lowers — only touched paths exist. It is the authoritative source of overlay state and, above all, the **cross-layer transaction boundary**: the per-node lock is the only place that can make an operation spanning independent backends commit atomically (§1.3.4).

```go
type graph struct {
    mu    sync.RWMutex        // guards the map structure ONLY; never held across I/O
    nodes map[string]*node    // key = clean slash path; sparse
    cow   singleflight.Group  // copy-up dedup, keyed by path
    store MetadataStore       // write-behind journal for whiteout/opaque (decision 8)
    // (optionally sharded: []shard{mu; nodes} keyed by path hash, to spread map contention)
}

type node struct {
    mu       sync.RWMutex // guards this node's fields
    path     string

    // masking state — authoritative; mirrored to the journal
    whiteout bool         // path (and subtree, in lowers) hidden
    opaque   bool         // dir present in top; lower children of it hidden

    // copy-up state — re-derivable from top.Lstat, so GC-able
    up       upState      // notCopied | copied  (the "copying" phase is owned by g.cow)

    // open-handle accounting — "what file is opened"
    handles  int          // live handles referencing this path
}
```

**The cross-layer atomicity invariant**

A reader resolving a path, and a writer mutating it, both take that path's node lock — so a reader sees the overlay state either fully *before* or fully *after* a write, never a torn intermediate where one backend has committed and another hasn't. The motivating case:

> **Remove a directory that is backed by a lower.** The dir shows in the merged view (top ∪ lower). `Remove` does two writes to two different systems: `top.Remove(dir)` (top fs) **and** set `whiteout` (graph bit + journal). An empty top is *not* the truth — without the whiteout, a concurrent reader falls through to the lower and sees the directory **resurrected**. Holding the node's write lock across both writes (and across the lower-lookup that decided a whiteout is even needed) makes the transition atomic from the overlay's view.

Whether a delete needs a whiteout at all is itself a cross-layer question — *"is this name still resolvable in a lower after removing it from top?"* — so that lower lookup must happen **under the same node lock** as the top-remove and the journal write. Lowers being read-only/static (decision 12) means the lookup can't race against lower mutation; the only races to defend against are *other overlay ops on the same path* and *top↔journal tearing*, both closed by the node lock.

**Why this removes the global lock**

- **Read fast path is graph-free.** Resolving an untouched path consults the layers directly (osfs/synthfs roots are read-safe); no node, no overlay lock. The graph is only touched when an ancestor or the path itself has a node — checked with O(depth) lookups on `g.nodes`.
- **Per-path mutation locks one node** (write) plus its **parent-dir node** when the child set changes (create/remove). Two goroutines on `a/x` and `b/y` never meet.
- **Copy-up never holds a lock across I/O.** It runs inside `g.cow.Do(path, …)` (`singleflight`), so concurrent writers to the same lower-only file dedup to a single copy; the node's `up` flips to `copied` when it returns.
- **The interning map lock (`g.mu`) is held only for map insert/delete**, never across filesystem I/O — so it serializes bookkeeping, not operations. Shard it if it shows up in profiles.
- **The journal is off the hot path.** Whiteout/opaque bits live on nodes; the `MetadataStore` is loaded once and written-behind on change.

**Lock ordering (deadlock avoidance)**

- Path resolution read-locks ancestor nodes hand-over-hand (parent → child), releasing as it descends.
- Structural single-path ops lock **parent before child**.
- Cross-path ops (`Rename`, `Link`) lock the two involved parent-dir nodes in **canonical lexical order**, then the child nodes.

**Authority precondition (decision 12 / OQ9):** the graph is trusted as truth because the overlay is the sole writer of `top` and the lowers are immutable for its lifetime. Copy-up state stays consistent; whiteout/opaque are only changed through the overlay.

### 5.1 Path resolution (the read path)

`Fs.resolve(name, followLast)`:
1. `filepath.Clean`; reject empty.
2. Resolve symlinks across the **merged** view using `fsutil.ResolvePath` over an `Lstat`/`ReadLink` pair backed by the overlay's own merged lookups (top-first, then lowers, honoring whiteout/opaque). `..` and absolute paths are rejected here → rootedness.
3. The resolved path is then looked up per-layer with no further symlink following.

Merged single-path lookup `lookup(name)` (used by Lstat/Stat/ReadLink/Open):
- Walk ancestors in the **graph** (§5.0): if any ancestor node (or the name's own node) is `whiteout` → `ENOENT`.
- Try `top.Lstat(name)`; if found, top wins.
- Else, for each lower high→low: skip lowers masked by an `opaque` ancestor node; return the first visible hit.

Masking is read from the graph's per-node bits (read-locked, hand-over-hand), never from the persisted store.

Because **lowers are immutable** (decision 12), a node may cache its resolved lower owner (which layer, what `FileInfo`) and a directory node may cache its merged lower dirents *for the overlay's lifetime* — not just per open handle as the old design did. Only top-side state needs revalidation. (Caching is an optimization, gated behind OQ8's GC policy; not required for v1 correctness.)

### 5.2 `MetadataStore` (the journal)

Because the node graph (§5.0) is the in-memory truth, the store carries **no query methods** — it only **loads once** at construction and **persists single changes** (write-behind). This is the reframe of decision 8 and resolves OQ6.

```go
type MetadataStore interface {
    // Load returns all persisted whiteout and opaque paths so the graph can be
    // seeded at construction. Called once. Paths are clean slash paths.
    Load() (whiteouts []string, opaques []string, err error)

    // Persist a single change. The graph already holds the truth; these durably
    // record it (write-behind, off the hot path). A directory whiteout masks its
    // whole lower subtree; opaque is exact-dir.
    SetWhiteout(name string) error
    ClearWhiteout(name string) error
    SetOpaque(dir string) error
    ClearOpaque(dir string) error
}
```

Subtree-masking and exact-opaque *queries* are answered by the graph (ancestor-node walk for whiteout; exact node for opaque), not the store. The default store keeps a flat in-memory set purely to support atomic full rewrites (§6); the authoritative tree is the graph.

### 5.3 Write path (copy-up + mutation)

For a mutating op on `name` (parent-dir node write-locked when the child set changes; target node write-locked):
1. Resolve.
2. If `name` resolves only to a lower layer (`top.Lstat` ⇒ `ENOENT`), `copyUp(name)` **inside `g.cow.Do(name, …)`** (singleflight, decision 11): ensure each missing parent dir exists in top (recursively, not opaque), then `CopyUpPolicy.CopyUp(…)` — the default stage policy writes content to a temp in the stage dir and **renames it into `name` within the same top root** (atomic publish, §5.6); flip the node's `up` to `copied`.
3. Perform the op on `top`.
4. Deletions (whole step under the node's write lock — §5.0 invariant): `Remove`/`RemoveAll` (a) `top.Remove` if present, (b) decide whiteout by a **lower lookup** — *is the name still resolvable in a lower?* — and if so set the node's `whiteout` bit + journal `SetWhiteout(name)`. An empty top alone is never the signal: a dir backed by a lower must be whited or it resurrects. Directory removal requires the **merged** dir to be empty first.
5. Recreation over a whiteout: a creating op (`Mkdir`/`Create`/`OpenFile O_CREATE`) at a whited node clears its `whiteout` bit + journals `ClearWhiteout`; `Mkdir` over a path that had lower content additionally sets `opaque` + journals `SetOpaque` (per OQ3).

Each state change updates the **node bit first** (under the node lock) and then write-behinds to the journal; the bit is the truth, the journal is durability.

### 5.4 Merged file & directory handle

- **Regular file (read):** opened from the single winning layer; no concatenation. (The old `overlayFile`/`layersFile` concatenation only ever mattered for directories — for a regular file the top-or-lower winner is opened directly. Simplify accordingly.)
- **Directory:** a merged handle that, on `ReadDir`/`Readdir`, unions dirents from top then each visible lower, **deduping by name (top wins)**, **dropping whited names**, and **not descending past an opaque dir into lowers**. Entries are sorted by name; the cursor model and `Seek` reset mirror the old `overlayFile` (lowers are assumed static, so the merged set may be cached per open handle).
- **Write-opened file:** after copy-up, the top file handle is returned directly (no overlay wrapper) — writes/Chmod/etc. act on top.
- **Handle accounting ("what file is opened"):** every successful open increments the node's `handles`; Close decrements. This is what backs Windows `ERROR_SHARING_VIOLATION` emulation (reject Remove/Rename of a path with live handles, à la synthfs `DisableOpenFileRemoval`) and keeps a copied-up node from being GC'd (OQ8) while open. Returned handles wrap the underlying layer file with a thin decrement-on-Close shim.

### 5.5 `OpenRoot` (sub-overlay)

`OpenRoot(sub)` returns a `*Fs` that **shares the same graph, top, lowers and stage dir**, and carries a `base = path.Join(parent.base, resolved-sub)`. Every method joins `base` with the caller's `name` (after confining `name` to not escape `base`) before touching top/lowers/graph/metadata. So there is no per-layer `OpenRoot` and no narrowing of the underlying roots — confinement is the overlay's own boundary check against `base` (mirroring synthfs's boundary dir): `..` past `base` is rejected, and cross-layer symlink resolution is bounded by `base`. Metadata uses `SubMetadataStore(meta, base)`; the graph stays keyed by full top-relative paths so parent and sub-overlay see consistent masking.

This base-prefix model (enabled by `Fs[File]` layers, decision 2) is why one stage dir at the top root serves every sub-overlay (§5.6).

### 5.6 Copy-up staging & atomic publish

Copy-up publishes a file by writing it to a temp and then renaming it into place. For that rename to be **atomic**, the temp and the destination must live on the **same filesystem** — which, given the single top `vroot.Fs[vroot.File]`, means both are paths **within that one top Fs**, so `top.Rename(temp, dest)` is a single rename → atomic. Both built-in policies satisfy this; they differ only in *where* the temp lives. **Neither hides anything** — the overlay has no reserved-name / namespace-filtering logic (decision 26).

**`CopyUpPolicyDotTmp` (default).** Writes `"<truncated-base>.<rand>.copyup.tmp"` **beside the destination** (same content dir → atomic `Rename`). No standing work dir; the temp is transient and renamed away immediately, so a completed listing is clean. A concurrent reader of that content dir may briefly glimpse the temp; the distinctive `.copyup.tmp` suffix bounds the blast radius and eases crash cleanup.

**`CopyUpPolicyStage` (opt-in, explicit work dir).** Writes the temp into an **explicit, caller-named work dir** at the top root (`NewCopyUpPolicyStage(workDir)`, default `.vroot-overlayfs.work`) and renames it to the full destination path. This corrals all scratch in one place instead of beside content.

- **Apparent, not hidden.** The work dir is a normal directory in the top; it **appears in the overlay's merged view** like any other entry. This is the deliberate, explicit model (Linux overlay's caller-designated `workdir`): the caller knows the dir exists and is responsible for choosing a name that doesn't collide with real content. If the caller doesn't want it surfaced (e.g. feeding `tar.AddFS`), they either use `DotTmp` or remove/ignore the work dir themselves.
- **One work dir, shared.** Because sub-overlays are base-prefix views over the *same* top Fs (§5.5), a single work dir at the top root serves all of them; copy-up always writes `<workDir>/<temp>` (full top path) and renames to the full destination.
- **Lazy + self-swept.** The policy creates the work dir on first copy-up and sweeps leftover `*.copyup.tmp` from a crashed run (distinctive suffix → safe glob).
- **Atomic publish.** `top.Rename(workDir/<temp>, name)` — one rename within the top Fs → near-atomic; a concurrent reader sees either the old (lower) or new (top) file, never a partial.

**Temp-name length & UTF-8 truncation.** The in-place policy derives the temp from the destination base name (for identifiability + collision-avoidance, with the random part guaranteeing uniqueness). A source file may have a ~`nameMax`-byte name, so `base + "." + rand(10) + ".copyup.tmp"` would overflow `NAME_MAX`. Build it as `fsutil.TruncateUTF8(base, nameMax-22) + "." + rand + ".copyup.tmp"` (22 = 10 random + 1 dot + 11 for `.copyup.tmp`), truncating at a rune boundary so no multi-byte sequence is split and the component stays ≤ `nameMax` bytes (the policy's ctor arg, default 255). The random suffix restores the uniqueness that truncation could otherwise collide away.

---

## 6. Persistent data shape

Two persisted artifacts per overlay:

1. **Top-layer content** — lives verbatim in the top `vroot.Fs[vroot.File]` (osfs dir on disk, memfs, …). Copy-up writes real files/dirs/symlinks there. No special encoding. The copy-up temps also land here: the default `DotTmp` policy writes transient `*.copyup.tmp` beside the destination; `CopyUpPolicyStage` (opt-in) keeps them in an explicit, **visible** work dir at the top root (§5.6). Neither is hidden — both are plain top content.

2. **Overlay metadata** — whiteouts + opaque dirs, **side-band** (OQ4a). Three built-in stores share the journal interface (decision 14); pick by durability need:

   | Store | Write cost / change | Durable | Use |
   |-------|---------------------|---------|-----|
   | `MetadataStoreMem` | O(1), none | no | ephemeral overlays (memfs top; feeding `tar.AddFS`) — **zero runtime cost** |
   | **`MetadataStoreLog`** (default durable) | **O(1) amortized** append + compaction | yes | the normal on-disk case (decision 23) |
   | `MetadataStoreSimpleText` | O(N) full rewrite | yes | tiny / max-inspectability; reference impl |

   **`MetadataStoreLog` — append-only journal + compaction (decision 23).** Why: `SimpleText` rewrites the whole file per change → O(N) write + an `fsync` each time → **O(N²)** for N deletions. An append-log makes each change one appended line (O(1) amortized); `Load` replays it; queries are answered by the graph, never the file. The chosen journal interface (`Load` + 4 persisters) is already change-shaped, so this is a drop-in store — no interface change.

   ```
   # vroot-overlayfs log v1
   +w "dir/removed.txt"     # set whiteout (subtree-masking)
   +o "recreated-dir"       # set opaque (exact dir)
   -w "dir/removed.txt"     # clear (tombstone)
   ```

   - Forward-slash paths, `strconv.Quote`d, `fs.ValidPath`-checked, never `.`/leading-`./`. Each append is a single full, newline-terminated line via one held-open `O_APPEND` handle. `Load` replays last-writer-wins and **tolerates a trailing partial line** (crash mid-append) by ignoring it. Unknown record types ignored (forward-compat).
   - **Compaction:** when `appendedLines > max(liveCount·factor, minThreshold)` (e.g. factor 2, min 64), rewrite the live set to `*.tmp` → fsync → rename → reopen append handle. The O(N) cost is amortized to O(1)/change. Compaction-time **normalization** drops records subsumed by an ancestor dir-whiteout/opaque, keeping the log and live set minimal (decision 25).
   - **Group commit (decision 24):** persisters enqueue a record to a single background writer (bounded channel); it batches appends and `fsync`s once per batch / debounce. Durability is a knob — `fsync`-per-op ↔ batched ↔ none. This takes persistence fully off the hot path; the writer serializes journal I/O without reintroducing any lock on filesystem ops. (Lifecycle via `errgroup`; flush on `Close`.)
   - **Crash consistency is unchanged** vs full-rewrite: the content↔metadata gap across a crash exists for any side-band store; the log is no worse.

   **Escape hatch:** at very large whiteout counts, a KV-backed store (SQLite/bbolt/pebble) gives O(log N) durable point writes — documented as a custom `MetadataStore`, not a default dependency. **In-band markers** (OQ4b: `.wh.<name>` + `.wh..wh..opq`) remain the "possible later" alternative; their one edge over side-band is inherently O(1)/change writes.

   Migration: old `whiteout` files (quoted paths, no type prefix) map cleanly to `+w` records; a one-shot reader can ingest them if any old-format stores exist (likely none post-rewrite).

---

## 7. File layout (proposed)

```
fsutil/
  truncate_utf8.go           TruncateUTF8 (rune-boundary byte truncation) + test

(vroot core: NO change — layers use existing vroot.Fs[vroot.File] + vroot.Widen)

vroot/overlayfs/
  doc.go                     package doc
  overlay.go                 Fs, New, Option, base-prefix OpenRoot, all vroot.Root methods
  graph.go                   graph: node interning (map/shards), singleflight copy-up, lock ordering
  node.go                    node: state bits (whiteout/opaque/up), handle accounting
  resolve.go                 merged resolve/lookup (graph-aware, hand-over-hand)
  layer.go                   Layer, NewLayer (erase file type via vroot.Widen)
  layers.go                  merged lower lookups + merged dir iteration
  file.go                    merged directory handle + handle-count shim (read path)
  metadatastore.go           MetadataStore (journal) interface + SubMetadataStore
  metadatastore_mem.go       MetadataStoreMem (ephemeral, no-op persistence)
  metadatastore_log.go       MetadataStoreLog — DEFAULT: append-log + compaction + group commit
  metadatastore_simple_text.go   minimal full-rewrite variant (reference)
  metadatastore_bench_test.go    N sequential removes: Log vs SimpleText
  copyuppolicy.go            CopyUpPolicy interface, ErrTypeNotSupported
  copyuppolicy_dot_tmp.go    CopyUpPolicyDotTmp (DEFAULT: in-place <base>.<rand>.copyup.tmp → Rename)
  copyuppolicy_stage.go      CopyUpPolicyStage (opt-in: visible work dir → atomic Rename; lazy-create + self-sweep)
  overlay_test.go            acceptancetest.RunRoot wiring
  copyup_test.go             copy-up / whiteout / opaque / cross-layer symlink / merged listing
  rename_test.go             rename across copy-up + whiteout-of-source
  concurrency_test.go        parallel ops on independent + same paths; -race; copy-up dedup
  acceptancetest/
    metadatastore.go         MetadataStore contract test (carry old + add opaque cases)
```

---

## 8. Test plan

- **Acceptance:** `acceptancetest.RunRoot[vroot.File, *overlayfs.Fs]` with `Make(t, lines)` building `top = memfs.New(...)` (or an osfs tmp dir) + a couple of empty lower layers, then materializing `lines` through the overlay's own write methods. `RunRoot` covers the full `Fs` suite + `OpenRoot` + escapes + race. Set `Option` skip flags per top capability (memfs supports Seek/ReadAt/WriteAt/symlink; Chown stored).
- **Overlay-specific:**
  - Resolution order (top wins; higher lower wins over lower lower).
  - Copy-up on write-open / Chmod / Chtimes / Rename / Link of a lower-only file; parent-dir copy-up.
  - Copy-up publish: default `DotTmp` leaves no standing artifact (post-op listings clean) and publishes via atomic same-Fs `Rename`; `CopyUpPolicyStage` work dir is **visible** in the merged view (no hiding), shared across sub-overlays, lazy-created, leftover `*.copyup.tmp` self-swept. (Acceptance `Make` uses `DotTmp` so strict listing assertions aren't tripped by a standing work dir.)
  - `fsutil.TruncateUTF8`: never splits a rune; result ≤ maxBytes; copy-up of a ~255-byte (multibyte) source name fits the DotTmp policy's `nameMax` and still publishes to the full original name.
  - Whiteout: delete a lower-only file → `ENOENT`, sibling still visible, listing omits it.
  - Opaque: `rm -rf dir && mkdir dir` over lower content → lower children hidden, new top children visible.
  - Recreation-over-whiteout clears whiteout (the old bug — regression test).
  - Cross-layer symlink resolution; symlink confinement (no escape).
  - Merged directory listing dedup + whiteout/opaque filtering.
  - `OpenRoot` sub-overlay correctness (sub-metadata, opaque inheritance).
- **Concurrency (the point of the node graph):**
  - `-race` stress: many goroutines mutating *independent* paths in parallel (must not serialize / must stay race-clean).
  - Concurrent writers to the **same** lower-only file → exactly one copy-up (singleflight), consistent final content.
  - Concurrent `Rename`s with crossing parents → no deadlock, consistent result (lock-ordering check).
  - Open-handle accounting under `DisableOpenFileRemoval`: Remove of an open path rejected; Close then Remove succeeds.
- **`MetadataStore` contract:** carry over `acceptancetest.MetadataStore` (whiteout cases) + opaque cases, run against **all three** stores (Mem/Log/SimpleText). Log-specific: append→`Load` replay (last-writer-wins); trailing-partial-line tolerance; compaction triggers + post-compaction normalization; group-commit flush on `Close`. Plus a benchmark of N sequential removes (Log should be ~linear total, SimpleText ~quadratic).
- **fs.FS conformance:** `fstest.TestFS(vroot.ToIoFs[vroot.File](fs), …)` over a populated overlay.
- `go test ./...` and `go test -race ./...`.

---

## 9. Implementation phases (sketch)

1. *(no vroot core change — layers use existing `vroot.Fs[vroot.File]` + `vroot.Widen`)*
2. `MetadataStore` (journal) + `MetadataStoreMem` + `MetadataStoreLog` (append+compaction+group-commit, default) + `MetadataStoreSimpleText` (variant) + `SubMetadataStore` + contract test + bench.
3. **Node graph** (`node.go`, `graph.go`): interning, per-node locks, `singleflight` copy-up, lock ordering, `Load`-seeding. Unit-test the graph in isolation.
4. `Layer` / `NewLayer` (+ layer merge lookups in `layers.go`).
5. `fsutil.TruncateUTF8`; `CopyUpPolicy` + `CopyUpPolicyDotTmp` (default) + `CopyUpPolicyStage` (opt-in, lazy-create + self-sweep its visible work dir).
6. `Fs`: graph-aware resolve/lookup (`resolve.go`), read path, merged dir handle + handle shim (`file.go`).
7. `Fs`: write path (copy-up via graph), Remove/RemoveAll (whiteout), Mkdir (opaque-on-recreate), Rename/Link/Symlink, Chmod/Chown/Chtimes.
8. `OpenRoot` sub-overlay.
9. Tests: acceptance + overlay-specific + concurrency/`-race`.
10. Update `vroot/README.md` overlay section to the generic API + node-graph notes.

---

## 10. Changelog

- 2026-06-17: **Removed `ReservedNamer` and all implicit work-dir hiding** per review (decision 26). The copy-up work dir is now an **explicit, visible** directory (Linux-`workdir` style) the caller designates — no overlay name-reservation/namespace-filtering. Consequence: a visible standing work dir is a poor default, so the **default policy flips to `CopyUpPolicyDotTmp`** (transient temps beside dest, no standing artifact); `CopyUpPolicyStage` becomes an explicit opt-in (decision 21, flagged reversible in §3). `DefaultStageDir`→`DefaultWorkDir`. Updated §4, §5.6, §6, §7, §8, phase 5, decisions 19/21/26.
- 2026-06-17: **Dropped the `CopyUpParams` struct** per review — `CopyUp` takes loose `(from, to vroot.Fs[vroot.File], name string)` args. With `StageDir`/`NameMax` already moved to the policy ctors, the struct held only 3 fields, isn't expected to grow, and the policy derives extras (e.g. `FileInfo`) from `from.Lstat(name)`; loose args match the old `CopyPolicy.CopyTo` and the rest of vroot's method style. Updated §4, §7, decision 26.
- 2026-06-17: **Moved staging config out of `Option`** per review (decision 26). `StageDir`/`NameMax` were policy-specific (meaningless when `CopyUpPolicyDotTmp` is chosen), so they now live on the policy ctors: `NewCopyUpPolicyStage(stageDir)` / `NewCopyUpPolicyDotTmp(nameMax)`, each owning + lazy-creating + self-sweeping its workspace. `Option` keeps only `CopyUpPolicy` + `DisableOpenFileRemoval`. The one real coupling (overlay must hide the stage dir) is now expressed by an optional `ReservedNamer{ Reserved() []string }` queried at `New`. `CopyUpParams` reduced to `{From,To,Name}`. Updated §4, §5.6, §6, decisions 19/22.
- 2026-06-17: **Dropped `RootFile`/`WidenRoot`** per review (OQ2 revised; decisions 2, 3). Layers are now stored as the existing **`vroot.Fs[vroot.File]`**, erased via the existing **`vroot.Widen`** — *zero* vroot core change, and the `Widen`→`WidenFs` rename is reverted. The overlay supplies its own `OpenRoot` via a **base-prefix over the shared graph** (§5.5) instead of delegating to a layer `OpenRoot`; confinement comes from rooted impls at runtime + the overlay's own boundary check. Consequence: **one** stage dir at the top root serves all sub-overlays, hidden only at the root (decision 19, §5.6 simplified). Updated §1.3.1, §4 (Layer/NewLayer/CopyUpParams/store ctors → `Fs[vroot.File]`), §6, §7, phase 1.
- 2026-06-16: Initial draft. Captured decisions 1–10 (recommended) and open questions OQ1–OQ6. Key shifts from the removed implementation: (2) introduce `vroot.RootFile`/`WidenRoot` to store heterogeneous rooted layers under generics; (5) split whiteout vs opaque and fix the delete-then-recreate bug; (6) filter whiteouts/opaque in directory listings; (8) extend the persistent metadata format to carry opaque records.
- 2026-06-17: Whiteout-store performance redesign per review. Root cause: `SimpleText` rewrites the whole file per change → O(N)/op, O(N²) for N deletes. Added **`MetadataStoreLog`** (append-only journal + threshold compaction + compaction-time normalization) as the default durable store — O(1) amortized writes, `Load`=replay, fits the existing journal interface unchanged (decisions 23, 25); optional **background writer + group commit** with a durability knob (decision 24); **`MetadataStoreMem`** for zero-cost ephemeral overlays; `SimpleText` demoted to a minimal/reference variant. Added `LogOption`, a store benchmark, and contract tests across all three stores. KV store / in-band markers documented as escape hatches.
- 2026-06-16: Copy-up redesign per review. Added a **stage area / workdir** (decision 19): a reserved subdir *inside the top root* so copy-up can publish via a single-root atomic `Rename` (same-superblock requirement; mirrors Linux overlay `workdir`); hidden at every path level, lazy-created, leftovers swept at `New` (§5.6). Renamed `CopyPolicy`→`CopyUpPolicy` / `CopyTo`→`CopyUp` (decision 20). Default is now **`CopyUpPolicyStage`** (decision 21); `CopyUpPolicyDotTmp` kept in-place but with a `.copyup.tmp` suffix and **UTF-8-aware base truncation** via new `fsutil.TruncateUTF8` to fit `NameMax`/255 (decision 22). Option gains `StageDir`, `NameMax`.
- 2026-06-16: **All remaining OQs resolved** (review + AskUserQuestion). OQ2 → core, and *rename `Widen`→`WidenFs`* + add `WidenRoot`/`RootFile`; OQ3 → opaque-on-recreate; OQ4 → side-band (in-band demoted); OQ5 → require writable top; OQ6 → journal interface (no queries); OQ7 → lazy graph; OQ8 → GC pure-cache nodes. Added decisions 13–18; §3 converted to a resolved log. Status → design settled.
- 2026-06-16: OQ9 resolved — lowers confirmed **immutable** for the overlay's lifetime (keystone of the atomicity invariant; also enables node-wide caching of lower lookups/dirents, §5.1). Sole-writer-of-top kept as a documented standard precondition; top existence revalidated per op (decision 12).
- 2026-06-16: Reframed the *purpose* of the node graph per review — from "drop the global lock for speed" to **cross-layer atomicity**: a logical op spans independent backends (lower/top/journal) that share no lock, so the per-node lock is the only possible transaction boundary (§1.3.4, decision 9, §5.0 invariant + remove-dir worked example, §5.3 deletion rule). Confirmed side-band whiteout/opaque metadata is safe because lowers are read-only (OQ4 leaning (a)).
- 2026-06-16: Concurrency pivot per review — drop the literal old-impl port; keep the ideas. Replaced the global `sync.RWMutex` with a **sparse in-memory node graph** as the overlay's authoritative deviation-state and lock unit (decision 9): per-node `sync.RWMutex`, `singleflight` copy-up dedup (decision 11), graph-as-truth + exclusive-top-ownership precondition (decision 12). Reframed `MetadataStore` as a load-once + write-behind **journal** (decision 8, §5.2, §6) — queries now answered by the graph. Node carries open-handle accounting ("what file is opened") backing `DisableOpenFileRemoval` and GC. Added OQ7 (lazy vs eager graph), OQ8 (node GC), OQ9 (exclusive top ownership), plus `node.go`/`graph.go`/`resolve.go`/`concurrency_test.go`.
