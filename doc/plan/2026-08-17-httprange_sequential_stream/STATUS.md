# STATUS — httprange sequential-stream lane

Current state: **planned and finalized** — decisions D1–D11 recorded
and traced through PLAN.md. The D10 gate reset was resolved by the
user's "Confirmed but slight modification. Remove Size.", folded as
D11 (delete `Size()`; `Metadata()` is the sole size exposure).
Implementation not started.

## Planning checklist

- [x] Idea questions resolved: D1 "lives inside httprange.ReaderAt …
  supersedes prior D7" ✓, D2 "closes the stream for good … never
  re-armed" ✓, D3 "let it fail … retry is caller's responsibility" ✓
- [x] Contract questions resolved: D4 "explicit Probe … or lazy" ✓,
  D5 "(off, n) … same rule as io.SectionReader … math.MaxInt64 for
  off-EOF" ✓, D6 "Add Config fields now" ✓
- [x] D7 "get metadata after or in-mid downloading" folded into
  IDEA/PLAN (Metadata type + accessor, step 1)
- [x] D8 "Probe … should be also a validator; … validate against
  actually fetched data on Probe time" — framing corrected across
  IDEA/PLAN (no behavior change; guard live only once a request
  happens, Probe the explicit act)
- [x] D9 "Probe is now lazy thing that also can be called explicitly.
  First Read also call Probe" — one probe routine, folded into
  IDEA/PLAN step 2 (trigger list since narrowed by D10)
- [x] D10 "specifying size should not make it lazy; … Probe is always
  lazy or explicit, not eager implicit" — construction does no I/O
  anywhere, `Size()` lazily settled, unsettled-size read handling;
  folded into IDEA/PLAN (steps 2, 4, 5, 6)
- [x] IDEA.md gate re-confirmed after D10 — "Confirmed but slight
  modification. Remove Size." ✓
- [x] D11 "Remove Size" — `Size()` deleted from the surface delta;
  `Metadata()` sole size exposure; folded into IDEA/PLAN (steps 2, 5,
  6)
- [x] Public surface delta written (PLAN.md)
- [x] Implementation steps detailed (PLAN.md steps 1–6)
- [x] Traceability walked: D1–D7 + inherited prior-D1 each mapped to
  steps (PLAN.md Traceability section); UC1–UC4 covered
- [x] IDEA.md gate re-confirmed by user after the D7 addition, with
  D5's section-view contract confirmed explicitly ✓

## Implementation checklist (mirrors PLAN.md steps — not started)

- [x] 1. Config validators + pin-or-verify + `Metadata()` (D6, D7)
- [x] 2. Probe as the single lazy/explicit verification (D4, D8, D9)
- [ ] 3. Stream lane in `stream.go` (D1–D4)
- [ ] 4. `NewRange` + `ReadAt` wiring, view semantics (D2, D3, D5)
- [ ] 5. Docs (`doc.go`, method docs)
- [ ] 6. Test matrix incl. request counts, races, resume mismatch

Next action: implementation step 1 (Config validators + pin-or-verify +
`Metadata()`), when the user asks for implementation.
