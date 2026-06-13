# stream — Improvement Plan 01 — Decisions

The user was unavailable during this work; decisions below were made by the
supervising agent on its own judgment, with multi-agent review evidence and
(where noted) codex consulted as an advisor.

## D0 — Scope: go.work modules only
`tarfs` and `aferofs` are NOT in `go.work` (pre-rewrite legacy, pinned to older
dependency versions). Improvements are scoped to the five active workspace
modules. Rationale: changes to non-workspace modules can't be verified against
the active build, and the rewrite branch direction supersedes them.

## D1 — File naming
The goal text spelled the decision file two different ways (`DICISION.md`,
`DESICION.md`); normalized to `DECISION.md`. "INDEX" in the plan dir name
interpreted as a sequence number → `01-IMPROVEMENT_PLAN`.

## D2 — seqReaderAt concurrency: internal mutex
Chosen: guard `ReadAt` with a `sync.Mutex`, keeping the `io.ReaderAt` claim
honest, rather than (a) renaming to a non-ReaderAt cursor type or (b) doc-only
warning. Rationale: trivial cost on the sequential hot path (uncontended lock),
removes a prose-only contract violation that composition (ChunkedSource →
MultiReadAtSeekCloser) silently inherits.

## D3 — Negative-size constructor inputs: panic
`NewMultiReadAtSeekCloser` (negative `SizedReaderAt.Size`) and `NewSeqReaderAt`
(negative `size`) panic with a clear message. Rationale: programmer error;
silent clamping hides bugs; constructors have no error return and adding one is
disproportionate churn.

## D4 — Drain-forward optimization: NOT taken
Reviewer proposed draining `off - r.off` bytes on forward jumps instead of
reopening. Rejected: changes observable open/transfer behavior with unclear
benefit (for ranged-HTTP backends a reopen can be cheaper than draining), and
the current reopen semantics are documented. Revisit only with profiling
evidence.

## D5 — EOF comparison: errors.Is
Bare `== io.EOF` replaced with `errors.Is(err, io.EOF)` when classifying errors
from user-supplied readers. Safety verified: the segment-size validation
(`read less` → io.ErrUnexpectedEOF) means a wrapped EOF can never mask a short
read, so the change strictly widens correct composition.
