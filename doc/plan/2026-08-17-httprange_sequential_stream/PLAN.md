# PLAN — httprange sequential-stream lane

`NewRange` — a section view over a remote object served by one streaming
range request until reads randomize, then by today's bounded per-`ReadAt`
requests — plus exported `Probe` and `Metadata`, and saved-validator
`Config` fields.

## Goal / success criteria

- `NewRange(ctx, url, off, n, cfg)` returns a section-view `*ReaderAt`
  over `[off, off+n)` whose in-order reads are served by a single
  `Range: bytes=off-…` request (IDEA.md UC1/UC2). A front-to-back
  `io.Copy` over it makes exactly one HTTP request.
- Construction never performs I/O (D10) — `New` and `NewRange` alike.
  Verification and size discovery happen lazily inside the reader's
  first request at no extra round trip, or explicitly via `Probe`;
  never eagerly at construction.
- A randomized read still succeeds, at one bounded request, and never
  blocks or fails because of the stream (UC3, D2).
- Caller-supplied metadata (`Config.Size`/`ETag`/`LastModified`, any
  subset) seeds what the reader knows — `Size` included, gating nothing
  — and is trusted until a request happens, validated the moment one
  does (D4/D6/D8/D9/D10): a resume caller probing first learns of a
  changed object there, as `ErrObjectChanged`, before any byte lands. The
  metadata to save for the next attempt is readable off the reader
  after or in the middle of downloading (D7).
- `ReadAt` semantics are unchanged for `New` callers; construction-time
  behavior changes deliberately (D10): `New` no longer probes, so a bad
  server or changed object surfaces at the first read or an explicit
  `Probe`. Existing tests asserting construction-time probing are
  adapted to the new failure point; all others pass untouched.
- `go test -race ./...` green, including concurrent mixed
  sequential/random reads against one reader.

## Scope

`stream/httprange` only: `NewRange`, exported `Probe` and `Metadata`,
`Config` validator fields, the internal stream lane, docs, and tests.

## Non-goals

- Chunk map, caching, request coalescing, idle auto-fetch — the rest of
  prior HANDOFF H1's `ChunkedStreamManager` shape stays future work
  (still recorded unclaimed in
  `doc/plan/2026-08-17-http_range_reader_at/HANDOFF.md`).
- Retry policy: stays with the caller's `Doer` (D3).
- Re-arming: one stream per reader; no second stream after a kill (D2) or
  a mid-stream failure (D3).

## Context

- `stream/httprange/httprange.go:167` — `New`; `:247` unexported
  `probe()` spends one `bytes=0-0` GET on size + range proof + meta
  pinning; `:311` `r.meta.Store(...)` unconditionally overwrites meta
  (must become pin-or-verify once `Config` can pre-pin, D6).
- `stream/httprange/reader_at.go:33` — `ReadAt` is stateless per call
  today; the stream lane adds shared mutable state that must not break
  the documented concurrency promise (`httprange.go:98`).
- `stream/httprange/reader_at.go:128` — `checkPartial` gates every 206;
  the streaming response passes an equivalent gate once, at open.
- `stream/httprange/reader_at.go:185` — `ifRangeValue` already reads
  pinned meta; pre-pinned caller validators ride it for free.
- `stream/httprange/httprange.go:229` — `newRequest` strips
  `Range`/`If-Range` from `Config.Header`, which is why saved validators
  need first-class fields (D6).
- Prior art: `doc/plan/2026-08-17-http_range_reader_at/` (D1, D7,
  HANDOFF H1, research doc §4/§7).

## Approach

One reader type, two lanes (D1). `NewRange` builds the same `*ReaderAt`
as `New` plus a stream-lane struct: the absolute base `off`, the view
length, and mutex-guarded stream state (body, absolute position, alive
flag). View arithmetic (relative→absolute, clamp, boundary EOF —
`io.SectionReader`'s rule, D5) happens at the top of `ReadAt`; then:

- offset == stream position and stream alive → lock, read from the body,
  advance. The check happens outside the lock (position stored
  atomically) so a randomized read never queues behind stream I/O.
- anything else → the stream is killed (once, permanently, D2/D3) and
  the read takes today's bounded path with the translated absolute
  offset.

Stream opening (D4 as amended by D10): always lazy — construction does
no I/O whatever `cfg` says; the stream opens under the lock on the
first in-order read, and callers wanting an earlier failure point call
`Probe` first. Verification is one routine with two triggers (D9/D10):
lazily inside the reader's first request (that request's response
serves as the probe's — no extra round trip), or an explicit `Probe`
call ahead of it. While the size is unsettled, first requests go out
unclamped and the response settles it: a 206's `Content-Range` total
pins the size (a response clamped at the object's end returns its
bytes plus EOF), a 416 with `bytes */N` pins it and reads as EOF, and
the empty-object 200-with-empty-body carve-out adopts size zero as the
old construction probe did. The streaming response passes the same gate
as any read: status
206 (416 with `bytes */N` = empty view), `checkContentEncoding`,
`Content-Range` start/total check, meta pin-or-verify, `If-Range` when
meta is pinned.

```mermaid
flowchart TD
    RA["ReadAt(p, off_rel)"] --> V{"view clamp:\noff_rel ≥ len → EOF\np clamped to boundary"}
    V --> S{"stream alive &&\nabs(off_rel) == pos?"}
    S -->|yes| L["lock; open body if lazy-pending;\nread; pos += n; unlock"]
    L -->|body error| K1["mark dead (D3);\nreturn partial + error"]
    L --> R1["return n (+EOF at boundary)"]
    S -->|no| K2["mark dead if alive (D2)"]
    K2 --> B["bounded Range request\n(today's path, absolute offset)"]
    B --> R2["return as today"]
```

Rejected shapes: separate wrapper type (Q1/D1); bypass-and-keep-open
(Q2/D2); transparent reopen (Q3/D3); `Window` struct with validator
fields (folded into `Config` instead, D6); whole-object reader with the
range as a mere hint (D5 — the user's `io.SectionReader` rule makes it
a view).

## Public surface delta

```go
package httprange

// ===================== Added =====================

// NewRange returns a ReaderAt over the section [off, off+n) of the
// remote object, as io.NewSectionReader does for a local one: offsets
// are relative to off, and reads hit EOF at the boundary. n larger
// than the remainder is clamped; use math.MaxInt64 to say "from off to
// the end" (archive/tar's idiom for io.SectionReader); n <= 0 yields
// an empty view. It is New, slightly optimized for reading the section
// mostly front to back: one streaming range request serves in-order
// reads until the first out-of-order read or stream error, after which
// every read is a bounded request of its own, as under New.
// Construction performs no I/O regardless of cfg (D10): the stream
// opens on the first in-order read and its response is validated as
// the probe's own; call Probe for an earlier failure point. For a full
// copy without knowing the size, wrap with
// io.NewSectionReader(r, 0, math.MaxInt64) and let EOF end the copy.
func NewRange(ctx context.Context, url string, off, n int64, cfg *Config) (*ReaderAt, error)

// Probe validates the reader's picture of the remote object against
// what the server actually has, right now: one GET Range: bytes=0-0
// that proves the server honours Range, checks every piece of metadata
// already held — caller-supplied Config fields (Size, ETag,
// LastModified, any subset) included — against the fetched response,
// failing with ErrObjectChanged on contradiction, and pins whatever
// was not yet known. Until some request happens (Probe, the stream
// opening, or a read), caller-supplied metadata is trusted, not
// verified; Probe is the explicit way to put that verification — and
// the failure point — here rather than at the first read. ctx bounds
// this call only.
//
// The same probe also runs lazily: when it was never called, the first
// request the reader makes — the stream opening or the first bounded
// read — doubles as it, its response validated the same way before any
// of its bytes are used, so the lazy path never costs an extra
// request. Once the probe has run, the reader's metadata is verified,
// and every later response is checked against it. Calling Probe on an
// already-verified reader fires and re-verifies.
func (r *ReaderAt) Probe(ctx context.Context) error

// Metadata is a snapshot of what the reader has pinned about the remote
// object. Size is the total size of the object (never a NewRange view's
// length) and, with ReaderAt.Size removed (D11), the only exposure of
// the size; zero with ok=false means not yet known.
type Metadata struct {
    ETag         string
    LastModified string
    Size         int64
}

// Metadata reports the pinned object metadata and whether the object's
// identity is settled yet — false until a response has been seen, when
// Config carried no metadata either. It is safe to call while reads
// are in flight, never blocks on them, and never issues a request of
// its own.
func (r *ReaderAt) Metadata() (Metadata, bool)

// ============ Changed (signatures unchanged) ============

// New no longer issues a probe at construction — construction never
// performs I/O (D10). Size discovery and range verification happen
// lazily at the first read, or explicitly via Probe. Errors New used
// to report at construction (unsupported ranges, bad status, changed
// object) surface there instead.
func New(ctx context.Context, url string, cfg *Config) (*ReaderAt, error)

// Config gains the two validator fields; the existing three are
// untouched, though Size joins ETag/LastModified as seeded metadata:
// trusted until a response verifies it, gating nothing about when
// requests happen (D10).
type Config struct {
    Client Doer        // unchanged
    Header http.Header // unchanged
    Size   int64       // unchanged in shape; see above

    // ETag and LastModified are validators the caller saved from an
    // earlier response, for resuming. They pre-pin the object identity
    // at construction and ride every request's If-Range (subject to
    // the same strong-validator rule as pinned ones) — but they are
    // trusted, not verified, until a request actually happens: the
    // first response (Probe's, the stream's, or a read's) is checked
    // against them and contradicts them with ErrObjectChanged before
    // any of its bytes are used. Call Probe to make that check happen
    // up front. Empty means none, as today.
    ETag         string
    LastModified string
}

// ==================== Removed ====================

// Deleted (D11): the size is exposed through Metadata() alone. With
// the size lazily settled (D10) a settled-at-construction accessor no
// longer exists. ReaderAt consequently stops satisfying
// stream.ReadAtSizeCloser — an interface living in
// stream/seq_reader_at.go, which the prior plan's D6 already slates
// for removal; multi-reader callers pass
// stream.SizedReaderAt{R: r, Size: m.Size} instead.
func (r *ReaderAt) Size() int64
```

Everything exported and not listed above is unchanged.
`NewRange(ctx, url, 0, math.MaxInt64, nil)` is `New` plus the
streaming lane.

## Implementation steps

1. **Config validators (D6)** — `httprange.go`: add `Config.ETag` /
   `Config.LastModified`; `New`/`NewRange` pre-pin
   `objectMeta{etag, lastModified}` when either is set. Change
   `probe()`'s unconditional `r.meta.Store` (`httprange.go:311`) and
   `checkPartial`'s CAS path (`reader_at.go:148-152`) into a shared
   pin-or-verify helper: pin fields that are empty, `ErrObjectChanged`
   on contradiction. In the same step add `Metadata`/`Metadata()` (D7)
   over the pinned state — the atomic `r.meta` load plus the settled
   size — since both features live on the same pin-or-verify plumbing.
   Judgment call: `ok` means the object's *identity* is pinned (a
   response seen, or `Config` carried a validator); the snapshot's
   `Size` is filled from `cfg.Size` even while `ok` is false, since a
   caller-supplied size is known without being pinned.
   Verifiable alone: resume-mismatch test (server with new ETag → first
   read fails), `ifRangeValue` sends the saved validator on the very
   first request, `Metadata()` reflects a Config pre-pin immediately
   and probe results after.
2. **Probe as the single lazy/explicit verification (D4, D8, D9)** —
   `httprange.go`: split today's `probe()` into the request half (the
   `bytes=0-0` GET) and an adopt-response half that runs every
   already-held metadatum — `cfg.Size` and the pre-pinned/pinned
   validators, each independently — through the step-1 pin-or-verify
   helper (`ErrObjectChanged` on any contradiction, pin what was
   empty) and flips a verified flag. The adopt-response half is what
   the lazy triggers reuse: the first bounded read and the stream open
   (steps 3–4) feed it their own response, so laziness costs no extra
   request; exported `Probe(ctx)` calls both halves explicitly and
   re-verifies when already verified. `New` stops calling it at
   construction (D10): construction does no I/O, and `ReadAt` learns
   to run before the size is settled — the request goes out unclamped,
   and the response settles it (206 `Content-Range` total; a response
   clamped at the object's end → bytes + EOF; 416 `bytes */N` → size
   pinned, EOF; the empty-object 200-with-empty-body carve-out → size
   zero). Delete the exported `Size()` method (`reader_at.go:111`,
   D11) — the size is exposed through `Metadata()` alone — and adapt
   its users in tests and docs. Existing tests asserting
   construction-time probing move to first-read/Probe assertions.
   Verifiable alone: no-request-at-construction test, probe-mismatch
   test, partial-metadata probes (only LastModified supplied → it is
   checked, ETag+size get pinned), first-read-verifies-lazily test,
   unknown-size read matrix (past-EOF → 416 → EOF; read clamped at
   end; empty object).
3. **Stream lane (D1/D2/D3/D4)** — new file `stream.go`: the lane
   struct (mutex, body, atomic absolute pos, alive/lazy-pending state),
   `openStream` building `Range: bytes=off-` or `bytes=off-(end-1)`,
   response gate (206 / 416-empty-view / 200+If-Range→`ErrObjectChanged`
   / 200→`ErrRangeIgnored`, `checkContentEncoding`, `Content-Range`
   start/total check, meta pin-or-verify from step 1), `kill()` closing
   the body via `drainAndClose`. No wiring into `ReadAt` yet.
   Verifiable alone: unit tests over the gate against a stub server.
4. **`NewRange` + `ReadAt` wiring (D5, D10)** — `httprange.go`/
   `reader_at.go`: constructor (validate `off >= 0`; no I/O — D10),
   lazy open on the first in-order read reusing step 2's
   unsettled-size handling (416 → empty view; `n <= 0` → empty view
   with no request ever), view translation at the top of `ReadAt`
   (relative offset,
   boundary EOF per `io.SectionReader` — the view's length stays
   internal now that `Size()` is gone, D11), lane
   fast-path check before the lock, fallback + permanent kill on
   mismatch or body error (partial bytes + error surfaced as today's
   `io.ErrUnexpectedEOF` wording), `Close` tearing down the body.
   Verifiable: UC1/UC2 request-count tests pass end to end.
5. **Docs** — `doc.go`: replace the "price is one HTTP round trip per
   ReadAt … give such a scan a buffer" paragraph's story with the two
   options (buffer, or `NewRange` when the range is known up front),
   and rewrite the construction story for D10 — `New` no longer
   probes; nothing does I/O until the first read or an explicit
   `Probe`; `Size()` is gone (D11) and the size is read from
   `Metadata()`. Method docs per the surface
   delta; note the concurrency story of the lane on the `ReaderAt` doc
   (`httprange.go:95-99`).
6. **Test matrix** — `stream/httprange`: request-count assertions
   (UC1 full copy = 1 request; UC2 resume = 1), fallback matrix (UC3:
   out-of-order read kills, later in-order reads stay bounded; killed
   lane never reopens), mid-stream death (UC4: partial bytes + error,
   later reads bounded), lazy-open with `Size` set (zero requests at
   construction), `Probe` early-failure, saved-validator resume
   mismatch, empty view (`off >= total`, `n <= 0`), `Metadata()` read
   mid-download and after (D7, including under `-race` alongside a
   streaming walker), race test mixing
   one sequential walker with random readers, and the zip round-trip
   (`zip_test.go`) rerun over a `NewRange(0, MaxInt64)` reader.

Each step compiles and passes `go test -race ./...` + `./govet.sh` (as
the prior plan used) before the next.

## Testing and verification

Covered per step above; the bar for done is the step-6 matrix green
under `-race`, the pre-existing suite untouched and green, and a manual
`io.Copy` smoke against a real HTTP server (Go `http.FileServer`)
observing exactly one access-log line for a full copy.

## Risks

- Shared stream state vs the documented concurrency promise: mitigated
  by the check-outside-lock design (step 4) and the race test (step 6);
  the failure mode to watch is a randomized read blocking behind a slow
  stream read.
- Stream-served bytes skip a per-read `Content-Range` check by nature;
  consistency rests on the open-time gate plus pinned meta. Bytes are
  still positionally exact because TCP ordering is the stream.
- A slow server holds the streaming connection for the reader's
  lifetime; `Close`/ctx cancellation must reliably abort it (step 4
  test: Close while a stream read is blocked).
- Lazy lane + concurrent first reads: two goroutines both at `pos` —
  the lock serializes opening; the loser sees `pos` advanced and falls
  back (acceptable: hint is performance-only).
- Unsettled-size reads (D10) add cases the old construction probe
  preempted — 416-as-EOF, end-clamped 206, empty-object 200 — now on
  the read path itself; each concurrent first read may run them at
  once, and all must adopt consistently through the pin-or-verify CAS.
- `New`'s failure point moves (D10): callers relying on construction
  errors need `Probe` — a doc-visible behavior change, called out in
  step 5.

## Open questions

None — Q1–Q6 resolved as DECISION.md D1–D6.

## Traceability

- D1 "lives inside httprange.ReaderAt" → steps 3–4.
- D2 "closes the stream for good … never re-armed" → step 4; proven in
  step 6 fallback matrix.
- D3 "let it fail … retry is caller's responsibility" → step 4 (no
  reopen path exists); step 6 death tests.
- D4 "Probe exported … eager when size unknown, lazy when known" →
  steps 2 (Probe), 3–4 (eager/lazy open); step 6 lazy/Probe tests.
- D5 "section view, io.SectionReader rule, MaxInt64 idiom, n<=0 empty"
  → step 4; documented in step 5; step 6 empty-view tests.
- D6 "Config.ETag/LastModified pre-pin, If-Range, ErrObjectChanged" →
  step 1; step 6 resume-mismatch test.
- D7 "get metadata after or in-mid downloading" → step 1 (`Metadata`
  type + accessor); documented step 5; step 6 mid-download tests.
- D8 "Probe … should be also a validator; … validate against actually
  fetched data on Probe time" → step 2; documented in the surface
  delta's Probe/Config comments; step 6 partial-metadata probe tests.
- D9 "Probe is now lazy … also can be called explicitly. First Read
  also call Probe" → step 2 (request/adopt-response split, verified
  flag), steps 3–4 (stream open and first bounded read feed the lazy
  trigger); step 6 first-read-verifies-lazily test.
- D10 "specifying size should not make it lazy; … Probe is always lazy
  or explicit, not eager implicit" → step 2 (`New` stops probing at
  construction, unsettled-size `ReadAt`),
  step 4 (`NewRange` constructor does no I/O); documented step 5;
  step 6 no-request-at-construction and unknown-size tests.
- D11 "Remove Size" → step 2 (delete `Size()`; `Metadata()` sole size
  exposure); documented step 5; step 6 adaptations.
- Inherited prior-D1 concurrency promise → step 4 design + step 6 race
  test.
- IDEA use cases: UC1/UC2 → steps 3–4, proven step 6 request counts;
  UC3 → step 4 + matrix; UC4 → step 4 + death tests.
