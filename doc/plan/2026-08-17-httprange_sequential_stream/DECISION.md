# DECISION — httprange sequential-stream lane

Inherited constraints quoted from
`doc/plan/2026-08-17-http_range_reader_at/DECISION.md`:

- D1 (prior plan, operative): "concurrent-safe only — every ReadAt a
  self-contained bounded range request … no shared mutable stream state."
  This plan's D1 (below) revisits the second half by user decision; the
  concurrency promise itself must survive untouched.
- D7 (prior plan, operative): "explicitly out of scope now and, if
  sequential performance matters later, belongs in a future explicit
  adapter or wrapper"; "**Rejected**: fusing an opportunistic sequential
  lane into the base reader". Superseded in part by this plan's D1.

## D1 — Stream lane fused into `ReaderAt`, superseding prior D7's rejection (2026-08-18, user)

**Choice**: the declared-range stream lane lives inside
`httprange.ReaderAt` (constructor `NewRange`, D5). This supersedes prior
D7's rejected-alternatives line for the caller-declared case.

**Rationale**: what D7 rejected was an *opportunistic* lane; this one only
exists when the caller declares it. Declaring at construction lets the
streaming request double as the probe, so the hint costs no extra round
trip; a wrapper would need a separate probe or an awkward size/meta
handoff plus a second exported type.

**Rejected**: a separate wrapper type honoring prior D7 literally.

## D2 — First randomized read kills the stream permanently (2026-08-18, user)

**Choice**: a `ReadAt` at any offset other than the stream's current
position closes the stream for good; that read and every later read take
today's bounded per-read path. One stream per reader, never re-armed.

**Rationale**: matches the request's own words ("fire single range request
until read is randomized") and keeps the state machine one-way. A caller
with a second sequential stretch builds a new reader.

**Rejected**: bypass — keeping the stream open for the next in-order read
while mismatches go bounded. More state, an idle-stream lifetime problem,
and the declared use cases (full download, resume) are strictly
sequential anyway.

## D3 — Mid-stream failure is final; retry is the caller's (2026-08-18, user)

**Choice**: when the streaming body errors or ends short, the failing
`ReadAt` returns the bytes it got plus the error, and the stream is dead.
The reader neither reopens the stream nor completes the failed read with a
bounded request. Later reads take the bounded path as with D2.

**Rationale** (user, verbatim): "Let it fail. Failed connection needs
explicit retry mechanism and it is caller's responsibility." Consistent
with the package's existing no-retry stance (retries belong to the
caller's `Doer`); recovery is the documented resume flow (IDEA.md UC2).

**Rejected**: transparently finishing the failed read via a bounded
request, or reopening the stream at the failure point.

## D4 — Exported `Probe`; `New` defaults unchanged; eager stream only when size is unknown (2026-08-18, user)

**Choice**: export `(*ReaderAt).Probe(ctx)` as an explicit verify-now
call (range proof, size cross-check, meta pin-or-verify). `New` keeps
today's default (probe unless `cfg.Size > 0`). `NewRange` never spends a
separate probe: with size unknown it opens the stream at construction
(the streaming response doubles as the probe); with `cfg.Size > 0` it
does no I/O at construction and opens the stream on the first in-order
read — "explicit Probe for clearer failure point detection, or lazy
called right before first Read call" (user).

**Rationale**: probing is not only about size — it pins metadata — but
when the size is set, pinning can equally happen from the first real
response; the caller chooses early failure (Probe) or fewest requests
(lazy).

**Rejected**: always-eager (loses `Size>0`'s no-I/O construction);
always-lazy (breaks `Size()`'s settled-at-construction contract when
size is unknown).

## D5 — `NewRange(ctx, url, off, n, cfg)`: a section view, `io.SectionReader`'s rules (2026-08-18, user)

**Choice**: `NewRange` takes `(off, n int64)` — `io.SectionReader`'s
argument shape, per the user's correction — and returns a *view* of
`[off, off+n)`: relative offsets, `Size()` = view length, EOF at the
boundary. `n` beyond the remainder clamps; `math.MaxInt64` is the
documented "off to EOF" idiom (user: archive/tar's practice with
`io.SectionReader`); `n <= 0` yields an empty view exactly as a negative
`n` does in `io.SectionReader` (EOF on first read). Documented as "New,
slightly optimized" — `NewRange(url, 0, math.MaxInt64, cfg)` ≡ `New`
plus the stream lane. Straddling-End semantics dissolve: the boundary is
simply the view's EOF.

**Rationale**: the user invoked "same rule as io.SectionReader"; the
view reading matches that rule, the `(off, n)` signature, and the use
cases (resume = `NewRange(url, n, math.MaxInt64)` + `io.Copy`, no
wrapping).

**Rejected**: whole-object reader with `(off, n)` as a mere streaming
hint (two boundary notions, wrapping still needed for resume); a
`Window` config struct and the name `NewSequential` (user: "Just use
(start, end int64). Name it NewRange"); `n < 0` meaning to-EOF (user:
"n < 0 in io.SectionReader works as io.EOF on first read attempt").

## D6 — Saved validators as `Config.ETag` / `Config.LastModified`, added now (2026-08-18, user)

**Choice**: two new `Config` fields carrying validators the caller saved
from an earlier response. When set they pre-pin the object identity at
construction: they ride `If-Range` (same strong-validator rule as pinned
ones) and any response contradicting them fails with `ErrObjectChanged`.
Serves `New` and `NewRange` alike.

**Rationale**: `newRequest` deliberately strips `If-Range` from
`Config.Header`, so without first-class fields a resuming caller cannot
make the reader verify the object is still the one their local bytes
came from — new-object bytes would silently splice onto stale local
bytes.

**Rejected**: deferring to a HANDOFF entry; validator fields on a
constructor-local struct (no such struct survives D5).

## D7 — Expose pinned metadata: `(*ReaderAt).Metadata()` (2026-08-18, user)

**Choice**: callers can read the object metadata after or in the middle
of downloading: `Metadata() (Metadata, bool)` with
`type Metadata struct { ETag, LastModified string; Size int64 }` — a
snapshot of what is pinned so far, `false` until anything is pinned
(possible only on a lazily-opened reader before its first request;
`Config` validators count as pinned). Safe to call concurrently with
reads. `Metadata.Size` is the total object size, which `NewRange`'s
view-length `Size()` no longer exposes.

**Rationale** (user): "we also need to be able to let callers get
metadata after or in-mid downloading" — it completes D6's resume loop:
save `Metadata().ETag`/`.LastModified` (plus how many bytes landed)
during this attempt, feed them back through `Config` on the next.
Shape notes (assistant judgment): a struct snapshot beats per-field
methods for atomicity of the pair; the pinned origin stays internal —
it is an anti-redirect-swap guard, not object identity a caller should
persist.

**Rejected**: individual `ETag()`/`LastModified()` methods; exposing
origin.

## D8 — Probe is a validator; the mutation guard is live only once a request happens (2026-08-18, user)

**Choice**: reframe D4/D6's mechanism (no behavior change): the
`Config` metadata fields (`Size`, `ETag`, `LastModified` — any subset)
are *trusted, not verified* until some request goes out. `Probe` is the
explicit validation act: "since it can accept partial metadata, it can
validate against actually fetched data on Probe time" (user) — each
supplied field is checked against the fetched response
(`ErrObjectChanged` on contradiction) and each missing one is pinned
from it. The recommended resume flow is therefore
`NewRange(…, cfg-with-saved-metadata)` → `Probe` → read; skipping
`Probe` defers the same check to whichever request happens first.

**Rationale** (user): the earlier goal wording — "a resume caller
passing saved validators gets ErrObjectChanged" — was "wrong framing":
it read as a passive property of the fields, when the guard exists
"only once Probe is actually happen[ed]" (or any first request).

**Rejected**: keeping the passive framing; making validation
Probe-only (first stream/read responses still verify, as before —
Probe just makes the point in time explicit).

## D9 — One probe, lazily run by the first request, explicitly callable (2026-08-18, user)

**Choice**: "Probe is now lazy thing that also can be called
explicitly. First Read also call Probe" (user, verbatim). Verification
is a single probe routine with three triggers: (1) the construction
request when the size is unknown (`New`'s `bytes=0-0`; `NewRange`'s
eager stream open — its response is the probe's); (2) lazily, invoked
by the reader's first request when construction skipped it — the first
bounded read or the lazy stream open feeds its *own* response to the
probe's validation, so the lazy path costs no extra round trip; (3)
explicitly, via `Probe(ctx)`, which fires its `bytes=0-0` request ahead
of any read (and re-verifies when called on an already-verified
reader). Until the probe has run, held metadata is trusted; after, it
is verified and every later response is checked against it.

**Rationale**: replaces D8's "the first response also verifies, as a
backstop" description with a single named mechanism — probe — instead
of validation logic scattered across paths. Keeps every request-count
property: lazy verification piggybacks on the request that was
happening anyway.

**Assistant judgment (noted for review)**: explicit `Probe` on a
`NewRange` reader fires the tiny `bytes=0-0` request rather than
opening the stream, because Probe's ctx bounds that call alone while
the stream must outlive it on the reader's own context; the stream
still opens on the first in-order read, carrying `If-Range` with the
now-verified validators. A probed resume therefore costs two requests
(tiny probe + stream) — the price of choosing the early failure point,
per the user's earlier framing ("explicit probe ahead of time or
efficient but fail on first read").

**Rejected**: first Read firing a separate probe request before its own
(doubles the first read's round trips for nothing the response itself
cannot prove); Probe-only validation with no lazy trigger (a caller
who never probes would get unverified splices — contradicts D6/D8).

## D10 — Construction never performs I/O; `Config.Size` gates nothing (2026-08-18, user)

**Choice**: "Now Config have all metadata including size, specifying
size should not make it lazy; even contrarily, Probe is always lazy or
explicit, not eager implicit" (user, near-verbatim). Construction —
`New` and `NewRange` alike — performs no I/O, ever. This supersedes
D4's "`New` keeps today's default (probe unless `cfg.Size > 0`)" and
the eager stream open, and amends D9's trigger list down to two: the
probe runs lazily inside the reader's first request (whose response is
validated as the probe's own — no extra round trip), or explicitly via
`Probe(ctx)`. `cfg.Size` becomes seeded metadata exactly like
`ETag`/`LastModified`: trusted until verified, gating nothing about
when requests happen.

**Consequences accepted with the choice**:
- `New`'s observable behavior changes: errors it used to report at
  construction (no range support, bad status, changed object) surface
  at the first read or an explicit `Probe`. Existing tests asserting
  construction-time probing move accordingly.
- `Size()` is no longer settled at construction: it reports the known
  value (from `Config` or the first response) and 0 while unknown;
  `Metadata()`'s ok distinguishes unknown from empty. Callers wanting
  it settled first call `Probe`.
- Reads may run before the size is settled: the request goes out
  unclamped and the response settles it — 206 `Content-Range` total
  pins the size (a response clamped at the object's end returns its
  bytes plus EOF), 416 `bytes */N` pins it and reads as EOF, and the
  empty-object 200-with-empty-body carve-out (prior plan D14) adopts
  size zero on the read path instead of the construction probe.
- The full-copy idiom without a known size is
  `io.NewSectionReader(r, 0, math.MaxInt64)` + `io.Copy`, ending at
  EOF — it needs no `Size()` call and keeps UC1 at exactly one
  request.

**Rejected**: keeping construction-time probing for `New` (the "eager
implicit" the user excluded); making `Size()` itself probe (blocking
network I/O inside an accessor that has no ctx and no error path).

## D11 — Remove `(*ReaderAt).Size()`; `Metadata()` is the sole size exposure (2026-08-18, user)

**Choice**: "Confirmed but slight modification. Remove Size." (user,
verbatim, on the D10 gate). The exported `Size()` method
(`reader_at.go:111`) is deleted. The size — total object size, never a
`NewRange` view's length — is exposed only through
`Metadata().Size`, valid once supplied via `Config` or settled by a
response. Internally the reader keeps its size state for clamping,
boundary EOF, and `Content-Range` verification; only the accessor
goes. This supersedes D7's remark that `Metadata.Size` compensates for
`NewRange`'s view-length `Size()` — there is no `Size()` of any kind
now.

**Rationale**: with D10 the method had become awkward — "0 while
unknown" with the real answer living in `Metadata()` anyway; one
exposure is enough. Removal consequences are contained:
`stream.ReadAtSizeCloser`, the one interface in this repo requiring
`Size() int64`, lives in `stream/seq_reader_at.go`, which the prior
plan's D6 records the user already intends to remove; the multi-reader
path takes size as a struct field
(`stream.SizedReaderAt{R: r, Size: m.Size}`), which `Metadata()`
feeds.

**Rejected**: keeping `Size()` as a convenience over `Metadata().Size`
(two exposures of one datum, one of them ambiguous at zero).
