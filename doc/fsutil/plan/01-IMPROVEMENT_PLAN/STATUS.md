# fsutil — Improvement Plan 01 — Status

| Entry | Title | Status | Commit |
|-------|-------|--------|--------|
| F1 | WrapPathErr/WrapLinkErr copy-on-write | done | - |
| F2 | copyEntry surfaces destination Close error | done | - |
| F3 | resumable part/sidecar dedup | done | - |
| F4 | SafeWriteOption struct-only hooks | done | - |
| F5 | decideResume pure-function extraction | planned | - |
| F6 | fs.ModeSymlink consistency | done (folded into F1 commit) | - |

Baseline: all module tests green (2026-06-13).
Per-entry summaries: `SUMMARY-<entry>.md` in this directory.
