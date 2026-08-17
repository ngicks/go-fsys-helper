# DECISION — httprange sequential-stream lane

Inherited constraints quoted from
`doc/plan/2026-08-17-http_range_reader_at/DECISION.md`:

- D1 (operative): "concurrent-safe only — every ReadAt a self-contained
  bounded range request … no shared mutable stream state." The stream lane
  necessarily revisits the second half; the concurrency promise itself
  must survive untouched.
- D7 (operative): "explicitly out of scope now and, if sequential
  performance matters later, belongs in a future explicit adapter or
  wrapper"; "**Rejected**: fusing an opportunistic sequential lane into
  the base reader". This plan is that "later"; whether the caller-declared
  lane may live in the base reader anyway is Q1.

## Stubs (pending)

- **Q1 — fused into ReaderAt (supersedes D7's rejection) vs separate
  wrapper.** Tentative: fused.
- **Q2 — randomized read kills the stream vs bypasses it.** Tentative:
  kill.
- **Q3 — mid-stream error: surface + permanent fallback vs transparent
  bounded completion.** Tentative: surface + fallback.
- **Q4 — declaration API shape** (unset-representation given start==0 is
  meaningful; saved-validator resume in or out).
- **Q5 — declared window with caller-supplied Size**: lazy vs eager stream.
- **Q6 — end-bounded windows now or later.**
