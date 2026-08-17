# HANDOFF — work leaving this plan

## H1 — Progressive/caching/coalescing reader (user-approved deferral, D7)

- **What**: a sequential-performance layer over `httprange.ReaderAt` (or
  any `io.ReaderAt`): kept-open progressive stream, chunk map with
  per-chunk dedup and contiguous-range coalescing — the
  `ChunkedStreamManager` shape mapped out in
  `research-PDF.js-random-access-reader-impl.md` §3/§4/§7.
- **Why not here**: explicit user decision (DECISION.md D7): "Later we'll
  add caching / sequential performance." Kept separate because it
  introduces stream ownership, synchronization, cancellation, and
  overlapping-download concerns that would break D1's stateless contract.
- **Follow-up**: a future plan designing it as an explicit wrapper reader;
  the research doc's citations are its starting material.
- **Picked up (partially)**: `doc/plan/2026-08-17-httprange_sequential_stream/`
  takes the kept-open-stream piece (caller-declared sequential window);
  chunk map / coalescing / auto-fetch remain unclaimed.
