# STATUS — HTTP range-read URL as io.ReaderAt

Current state: **implemented** — all six steps done, reviewed
(approve-with-nits; nits fixed), `go test -race ./...` and `./govet.sh`
green. Post-plan decisions D12–D15 recorded (empty-object probe
carve-out, strong-validator If-Range, errors.AsType + go 1.26 bump).

## Decision checklist

- [x] D1: concurrent-safe only — every ReadAt a self-contained bounded
  range request ✓ (delivered by PLAN steps 2–3, race-tested in step 4)
- [x] D2: new `stream/httprange` package ✓ (step 2)
- [x] D3: ctx captured at construction, `Close` cancels it — recorded
  rule deviation ✓ (steps 2–3, cancellation tests in step 4)
- [x] D4: caller-supplied size else `GET Range: bytes=0-0` probe,
  `Content-Encoding: identity` guard, reject unknown size ✓ (step 2,
  probe matrix in step 4)
- [x] D5: If-Range validators + Content-Range total check, sentinel
  `ErrObjectChanged`, no strictness knob ✓ (step 3, mutation tests in
  step 4)
- [x] D6: no reliance on the removed sequential offset-opener reader ✓
  (verified by grep over plan; nothing in steps builds on it)
- [x] D7: progressive/caching layer deferred to future explicit wrapper ✓
  (PLAN non-goals)
- [x] D8: self-contained package, no reliance on other packages' helpers ✓
  (step 1 implements redaction/parsing in-package)
- [x] D9+D11: unexpected statuses → `*StatusCodeError` with
  `NotFound() bool` (404/410), NOT wrapping `fs.ErrNotExist` ✓
  (steps 2, 4)
- [x] D10: 200-to-ranged-request → `ErrRangeIgnored`, one sentinel for
  probe and mid-read ✓ (steps 2, 3, 4)

## Implementation checklist (mirrors PLAN.md steps)

- [x] 1. In-package helpers — URL-secret redaction + Content-Range
  parsing incl. `bytes */N` (self-contained per D8)
- [x] 2. `httprange.New` + probe flow (D2/D3/D4) — plus D14 carve-out:
  empty-body 200 accepted as zero-size object
- [x] 3. `ReaderAt.ReadAt/Size/Close` (D1/D3/D5) — If-Range sent only
  with a strong validator (D13)
- [x] 4. httptest matrix incl. `-race` parallel ReadAt (D1), mutation →
  `ErrObjectChanged` (D5), probe cases (D4), cancellation (D3)
- [x] 5. zip round-trip (UC1) + `SizedReadersFromReadAtSizer` compile
  check (UC3)
- [x] 6. `stream/README.md`, `go test ./...`, `./govet.sh`

## Research

- `research-PDF.js-random-access-reader-impl.md` — how PDF.js turns a URL
  into a random-accessible reader (3 parallel explorer agents over the
  mozilla/pdf.js master clone). Fed D4 (probe choice, Content-Encoding
  guard) and D5 (PDF.js has no change detection; origin pinning noted).

## Traceability (decision clause → owning step)

- D1 "per-ReadAt bounded request, concurrent-safe" → steps 3, 4
- D2 "new stream/httprange" → step 2
- D3 "ctx at construction / Close cancels" → steps 2, 3, 4
- D4 "probe, size skip, identity guard, reject unknown" → steps 2, 4
- D5 "validators + size, sentinel error, no knob" → steps 3, 4
- D6 "no reliance on removed reader" → constraint on all steps (none use it)
- D7 "defer progressive/caching wrapper" → non-goals (no step; deferred by
  user decision, not silent drop)
- D8 "self-contained, in-package helpers" → step 1
- D9/D11 "*StatusCodeError w/ NotFound(), no fs.ErrNotExist" → steps 2, 4
- D10 "ErrRangeIgnored, unified probe/mid-read" → steps 2, 3, 4
- UC1 (remote zip) → step 5; UC2 (tarfs) → steps 3+4 (bounded reads,
  caller buffering documented); UC3 (multi-reader segment) → step 5;
  UC4 (auth/redaction) → steps 2, 4

## Handoff

- HANDOFF.md H1 — progressive/caching layer, user-approved deferral (D7).

## Next action

None — implementation complete. Deferred: progressive/caching wrapper
(HANDOFF.md H1); optional backport of the safer URL redaction fallback
to `stream/fileserver` (its parse-failure path echoes the raw URL).
