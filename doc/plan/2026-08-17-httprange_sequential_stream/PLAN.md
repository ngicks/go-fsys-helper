# PLAN — httprange sequential-stream lane

One streaming range request serves a caller-declared sequential window;
reads outside the window fall back to today's bounded per-`ReadAt` requests.

**Skeleton only — the idea gate (IDEA.md) has not passed. Contracts and
implementation steps are detailed after it does.**

## Goal / success criteria

- A caller declaring "read from `start` to end" gets that stretch served by
  a single `Range: bytes=<start>-` request for as long as reads arrive
  sequentially (IDEA.md UC1/UC2).
- Declaring the window replaces the construction probe — no extra round
  trip versus today's `New` (IDEA.md usability).
- A randomized read still succeeds, at one bounded request, and never
  blocks or fails because of the hint (UC3).
- Undeclared behavior is unchanged; all existing tests keep passing.
- `go test -race ./...` green with concurrent mixed sequential/random reads.

## Scope

- `stream/httprange` only: `Config`/constructor surface for declaring the
  window, the internal stream lane, its fallback, docs, and tests.

## Non-goals

- Chunk map, caching, request coalescing, idle auto-fetch — the rest of
  HANDOFF H1's `ChunkedStreamManager` shape stays future work.
- Retry policy: stays with the caller's `Doer`.
- Re-arming: opening a *second* stream after fallback (pending Q2/Q3
  resolution; tentatively out).

## Context

- `stream/httprange/httprange.go:167` — `New` builds the reader; `:247`
  `probe()` spends one `bytes=0-0` GET on size + range proof + meta pinning.
  A declared window's `bytes=<start>-` response carries the same facts.
- `stream/httprange/reader_at.go:33` — `ReadAt` is stateless per call
  today; the stream lane adds shared mutable state that must not break the
  documented concurrency promise (`httprange.go:98`).
- `stream/httprange/reader_at.go:128` — `checkPartial` gates every 206
  (Content-Range shape, meta match); the streaming response must pass the
  same gate, with `end` open-ended.
- `stream/httprange/httprange.go:33` — `Config`: `Size int64` already uses
  "zero means unset"; a window `start` of 0 is meaningful, so the window
  field cannot use that convention (see open questions).
- Prior art: `doc/plan/2026-08-17-http_range_reader_at/`
  (D1, D7, HANDOFF H1, research doc §4/§7).

## Approach

To be detailed after the idea gate. Sketch: construction with a declared
window issues `Range: bytes=<start>-` instead of the probe, keeps the body,
and records `pos = start`. `ReadAt(p, off)` with `off == pos` (and the
stream healthy) reads from the body under a lock and advances `pos`; any
other read takes today's bounded path. Q2/Q3 decide when the stream is
closed for good. Offset check happens before committing to the stream lock
so randomized reads do not serialize behind stream I/O.

## Public surface delta

To be written after the idea gate (Q1 decides whether this lands on
`Config`/`ReaderAt` or on a new wrapper type).

## Implementation steps

To be written after the idea gate.

## Testing and verification

To be detailed; at minimum: request-count assertions for UC1/UC2 (exactly
one request), fallback matrix for UC3, mid-stream-death behavior for UC4,
meta/Content-Range gating of the streaming response, race tests mixing
sequential and random readers, and the existing suite untouched.

## Risks

- Shared stream state vs the documented concurrency promise (deadlock or
  serialization of unrelated reads).
- Bytes served from the stream bypass `checkPartial`'s per-read
  Content-Range check by construction — consistency rests on the single
  response's initial validation plus pinned meta.
- A server that streams slowly holds a connection open for the reader's
  lifetime; Close/ctx cancellation must reliably tear it down.

## Open questions

Idea-level (gate blockers):

1. **Fused vs wrapper — supersedes D7?** Prior D7 rejected "fusing an
   opportunistic sequential lane into the base reader"; this lane is
   caller-declared, not opportunistic. Options: (a) fused — a window hint
   on `Config` (or `New` variant), enabling stream-as-probe, one type to
   hold; DECISION.md records it as superseding D7's rejection. (b) wrapper
   — a separate type honoring D7 literally; costs a separate probe or an
   awkward size handoff, and two types. **Tentative default: (a) fused.**
2. **What does a randomized read do to the stream?** Options: (a) kill —
   first non-sequential read closes the stream permanently; matches the
   user's words ("until read is randomized"), simplest state machine; but
   e.g. one tail read (zip central directory) kills a start=0 stream
   instantly. (b) bypass — mismatched reads take the bounded path while
   the stream stays open for the next in-order read; tolerant, more state.
   **Tentative default: (a) kill.**
3. **Mid-stream error (UC4).** Options: (a) surface the error with partial
   bytes, later reads permanently bounded — consistent with no-retry
   stance, resume (UC2) is the recovery story. (b) transparently finish
   the failed read via a bounded request. **Tentative default: (a).**

Contract-level (after the gate):

4. Window declaration shape: field vs `New` variant; how "unset" is
   expressed given `start == 0` is meaningful (pointer, bool+offset,
   separate constructor); naming. Whether the caller may also hand in
   saved validators for resume (UC2) or that stays out of scope.
5. Whether a declared window with `cfg.Size > 0` also skips the probe and
   opens the stream lazily vs at `New`.
6. End-bounded windows (`start` to `end`, not to EOF): support now or
   keep windows open-ended only.
