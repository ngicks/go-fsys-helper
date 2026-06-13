# vroot — Improvement Plan 01 — Status

| Entry | Title | Status | Commit |
|-------|-------|--------|--------|
| V1 | acceptance escape-suite expansion (all methods, real targets) | done | - |
| V2 | synthfs Rename subtree-cycle rejection | done | - |
| V3 | synthfs mode ownership + uniform removal guard | done | - |
| V4 | WalkDir sibling-loop fix + escape-safety docs | done | - |
| V5 | internal/paths iterator hardening | done | - |
| V6 | sftpfs error-convention conformance + doc fix | planned | - |

Baseline: all module tests green (2026-06-13).
Per-entry summaries: `SUMMARY-<entry>.md` in this directory.

Order note: V1's "dir-into-own-subtree rename" acceptance vector was deferred to
V2 (it fails against synthfs until the V2 fix lands); it is added together with
the synthfs Rename fix in the V2 commit.
