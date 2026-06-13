# vroot — Improvement Plan 01 — Status

| Entry | Title | Status | Commit |
|-------|-------|--------|--------|
| V1 | acceptance escape-suite expansion (all methods, real targets) | done | 5aa4294 |
| V2 | synthfs Rename subtree-cycle rejection | done | a2d2402 |
| V3 | synthfs mode ownership + uniform removal guard | done | 8b7dc28 |
| V4 | WalkDir sibling-loop fix + escape-safety docs | done | 29b751c |
| V5 | internal/paths iterator hardening | done | 8e5618c |
| V6 | sftpfs error-convention conformance + doc fix | done | 67b7e4e |

Baseline: all module tests green (2026-06-13).
Per-entry summaries: `SUMMARY-<entry>.md` in this directory.

Order note: V1's "dir-into-own-subtree rename" acceptance vector was deferred to
V2 (it fails against synthfs until the V2 fix lands); it is added together with
the synthfs Rename fix in the V2 commit.
