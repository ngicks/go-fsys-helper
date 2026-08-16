# DECISION — HTTP range-read URL as io.ReaderAt

Append-only log. One entry per material decision: choice, rationale,
rejected alternatives.

## D1 — Concurrency contract: concurrent-safe only (2026-08-17, user)

**Choice**: one implementation; every ReadAt issues its own bounded
`Range: bytes=from-to` request. Fully honors the stdlib `io.ReaderAt`
parallel-call contract, no shared mutable stream state.

**Rationale**: the primary consumers (zip, `NewMultiReadAtSeekCloser`
segments) assume the stdlib contract; one honest implementation beats two
variants with a misuse trap. Sequential scans paying a request per read
chunk is accepted; callers mitigate with `io.NewSectionReader` +
`bufio.Reader`.

**Rejected**: (a) a sequential-only kept-open-stream reader — violates the
contract consumers assume; (b) shipping both a sequential and a concurrent
variant — larger surface with a misuse trap, and caller-side buffering
(`io.NewSectionReader` + `bufio.Reader`) covers the sequential case.

## D2 — Placement: new `stream/httprange` package (2026-08-17, user)

**Choice**: a new sibling package `stream/httprange` taking a plain URL.

**Rationale**: clean, single identity — "HTTP range requests → ReaderAt" —
in its own package, self-contained (see D8).

**Rejected**: (a) folding it into an existing package — muddies that
package's identity and couples lifecycles; (b) top-level `stream` — pulls
net/http into a package that is generic io glue today.

## D6 — Do not build on the existing sequential offset-opener reader (2026-08-17, user)

**Choice**: the plan must not reference or rely on `stream`'s existing
sequential offset-opener ReaderAt (`stream/seq_reader_at.go`) — the user
intends to remove it. All plan references to it were removed; the sequential
use case is served by caller-side buffering over the new concurrent-safe
reader instead. Its removal itself is a separate effort, not in this plan's
scope.

## D8 — No reference to `stream/fileserver`; httprange is self-contained (2026-08-17, user)

**Choice**: the plan must not reference or build on `stream/fileserver`.
All plan references were removed; instead of extracting shared helpers into
an internal package, `httprange` implements URL-secret redaction and
Content-Range parsing as its own unexported helpers. No existing package is
touched by this plan.

## D9 — 404/410 map to explicit `ErrNotFound`, not `fs.ErrNotExist` (2026-08-17, user; sentinel shape superseded by D11)

**Choice**: 404 and 410 responses fail with the package sentinel
`httprange.ErrNotFound`, which deliberately does NOT wrap `fs.ErrNotExist`.

**Rationale** (user): wrapping `fs.ErrNotExist` may confuse downstream
consumers — code that branches on `fs.ErrNotExist` typically assumes a
filesystem-shaped source, and a remote HTTP object is not one.

**Rejected**: wrapping `fs.ErrNotExist` (the earlier draft's convention) —
convenient for fs-generic callers but ambiguous for everyone else.

## D11 — Unexpected statuses become `*StatusCodeError` with `NotFound() bool` (2026-08-17, user)

**Choice**: `ErrNotFound` (D9) and the generic unexpected-status error
merge into one typed error, `*StatusCodeError{Code int}`, with a
`NotFound() bool` method reporting 404/410. Matched with `errors.As`; the
raw code is available for any other branching. D9's operative stance is
inherited unchanged: it does "NOT wrap `fs.ErrNotExist`" even when
`NotFound()` is true.

**Rationale** (user): one error shape for all status failures; callers get
the not-found verdict as a method instead of a second sentinel, plus the
actual code.

**Rejected**: separate `ErrNotFound` sentinel + anonymous unexpected-status
errors (D9's shape) — two shapes for one kind of failure, and the
non-404 codes were unbranchable.

## D10 — Named sentinel `ErrRangeIgnored` for 200-to-ranged-request (2026-08-17, user + judgment call)

**Choice**: a 200 answer to a ranged request fails with the sentinel
`httprange.ErrRangeIgnored` (user asked for the mid-read case to be named).
Judgment call on top: it REPLACES the draft's construction-time
`ErrRangeNotSupported` rather than joining it — probe-time and mid-read
200s are the same observable event (server ignored the Range header), and
two sentinels would force consumers to check both for one condition. The
error message distinguishes where it happened. The If-Range-triggered 200
remains `ErrObjectChanged` (D5), since there the 200 is HTTP's
entity-changed signal, not range refusal.

**Rejected**: keeping `ErrRangeNotSupported` alongside `ErrRangeIgnored` —
two names for one observable; an unnamed mid-read error — callers cannot
branch on it.

## D3 — Context captured at construction (2026-08-17, user)

**Choice**: the constructor takes `ctx context.Context`; every HTTP request
of the reader's lifetime derives from it, and `Close` cancels the derived
context. **This is an explicit, recorded deviation from the repo rule "do
not stash a context in a struct"** — accepted because `ReadAt` has no ctx
parameter, mirroring how `http.Request` carries a ctx through a value's
lifetime; one ctx bounds the whole session.

**Rejected**: (a) `WithContext(ctx)` derived views — every consumer handoff
(zip, tarfs) must remember to call it or requests run unbounded; (b)
`context.Background()` + client timeouts only — a hung server blocks ReadAt
indefinitely.

## D4 — Size discovery via ranged-GET probe (2026-08-17, user)

**Choice**: if the caller supplies the size, skip discovery (range-support
failures then surface on the first ReadAt instead of at construction).
Otherwise the constructor issues a single `GET Range: bytes=0-0` probe:
a 206 response proves range support directly, its `Content-Range` total is
the size, and `Content-Encoding` must be absent or `identity` (guard
adopted from PDF.js — compressed transfer makes offsets/length
untrustworthy). Unknown size is always a construction error.

**Rationale** (user): the plain-GET-headers approach (PDF.js's shape) has
no request-count advantage when its body is immediately closed, does not
prove range support (`Accept-Ranges: bytes` is advisory), and may waste a
connection/bandwidth. The ranged probe answers both questions in one
round trip.

**Rejected**: (a) HEAD — requires Content-Length on HEAD, proves nothing
about ranges, and a server failing the probe is unusable for us anyway, so
a HEAD fallback adds nothing; (b) plain GET from offset 0 with the body
closed after headers — see rationale.

## D7 — Sequential/progressive performance deferred to a future wrapper (2026-08-17, user)

**Choice**: the base `httprange` ReaderAt stays stateless and
concurrent-safe per D1. A PDF.js-style progressive stream / caching /
coalescing layer (kept-open head stream served opportunistically, chunk
map, request coalescing) is explicitly out of scope now and, if sequential
performance matters later, will be designed as a separate explicit reader
or wrapper — separate because it introduces stream ownership,
synchronization, cancellation, and overlapping-download concerns.

**Rejected**: fusing an opportunistic sequential lane into the base reader
— reintroduces shared mutable state and locking into the D1 contract.

## D5 — Change detection: validators + size, not configurable (2026-08-17, user)

**Choice**: record ETag/Last-Modified from the probe response; send
`If-Range` on every ReadAt request and compare returned validators when
present; always compare the response's `Content-Range` total against the
recorded size. Any mismatch (including a 200 answer to an `If-Range`d
request, which is HTTP's "entity changed" signal) fails with a distinct
sentinel error. No strictness knob.

**Rationale**: PDF.js's do-nothing approach silently splices bytes from two
object versions (research doc §5) — the exact failure IDEA.md rejects; a
knob to reopen that hole isn't worth the surface.

**Rejected**: (a) size-only check — misses same-size content swaps; (b)
configurable strictness — escape hatch that reintroduces silent corruption.

**Addendum — origin pinning (judgment call, not user-asked)**: adopted. The
post-redirect origin of the probe response is recorded and every ReadAt
response's final origin must match, failing with `ErrObjectChanged`
otherwise. With a redirect-following Doer, successive requests can land on
different hosts mid-session; a swapped host serving a same-size object with
no validators would otherwise pass. One comparison per response, borrowed
from PDF.js's only cross-request guard (research doc §5/§7).

**Addendum — probe-skip path (judgment call)**: when `cfg.Size > 0` skips
the probe (D4), validators and origin are captured lazily from the first
ReadAt's 206 response, at zero extra requests — so change detection is only
size-based for that single first response, full-strength afterwards.

## D12 — Per-step commits may be intermediate [automatic] (2026-08-17)

**Choice**: the user asked for a commit after each implementation step, so
the step-2 commit lands with `New`/probe present but `ReadAt` not yet
implemented (methods land in step 3). Each such commit compiles and passes
the tests that exist at that point; it is not a finished reader until the
step-3 commit.

**Rationale**: honors "commit after each step" literally; noting it here so
an intermediate commit is not mistaken for a complete feature.

## D13 — If-Range only with a strong validator [automatic] (2026-08-17)

**Choice**: `If-Range` is sent only when a usable validator exists per RFC
9110: a strong ETag (no `W/` prefix), else Last-Modified when present. A
weak ETag is never sent in `If-Range` (compliant servers must ignore it);
change detection then relies on comparing the response's ETag /
Content-Range total against the recorded values, which stays in force in
all cases.

**Rationale**: sending a weak validator in `If-Range` would be silently
ignored by compliant servers, misclassifying object changes; the
response-side comparisons keep detection honest regardless.

## D14 — Probe accepts empty-body 200 as a zero-size object [automatic] (2026-08-17)

**Choice**: the construction probe treats a 200 response with no
`Content-Range` and `Content-Length: 0` as a zero-size object (size 0,
validators/origin captured), instead of failing with `ErrRangeIgnored`.

**Rationale**: Go's own `http.ServeContent` deliberately answers a range
request on an empty file with 200, not 416 (`net/http/fs.go`, the
`errNoOverlap` + `size == 0` branch) — so a strict probe would reject empty
objects behind every standard Go file server. The carve-out cannot cause
corruption: a `Content-Length: 0` 200 body IS the whole (empty)
representation, and a zero-size reader never issues a ranged request —
every ReadAt returns EOF locally. Range support remains unproven for such
objects, but it is also never needed.

**Rejected**: strict spec-only 416 `bytes */N` handling — punishes the
most common Go server implementation for a case where range semantics are
irrelevant.

## D15 — Use errors.AsType; stream module bumped to go 1.26 (2026-08-17, user)

**Choice**: all typed-error matching in `httprange` (code and tests) uses
`errors.AsType[T]` instead of `errors.As`. Since `errors.AsType` was added
in Go 1.26, the `stream` module's `go` directive rises from `1.22.0` to
`1.26.0` (the sibling `vroot` module already declares 1.26.x).

**Rationale**: user instruction; the generic form is type-safe and drops
the target-pointer boilerplate.
