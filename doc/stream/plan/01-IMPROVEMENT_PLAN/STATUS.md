# stream — Improvement Plan 01 — Status

| Entry | Title | Status | Commit |
|-------|-------|--------|--------|
| S1 | errors.Is EOF + constructor validation | done | f36dd01 |
| S2 | MultiReadError index fix + segment-read dedup + Error() fields | done | a0dafd4 |
| S3 | seqReaderAt mutex + reopen-path hardening | done | 0c05026 |
| S4 | close fan-out + drainClose dedup | done | 0ea2fbc |

Baseline: all module tests green (2026-06-13).
Per-entry summaries: `SUMMARY-<entry>.md` in this directory.
