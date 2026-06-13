# fsutil — Improvement Plan 01 — Status

| Entry | Title | Status | Commit |
|-------|-------|--------|--------|
| F1 | WrapPathErr/WrapLinkErr copy-on-write | done | 7612715 |
| F2 | copyEntry surfaces destination Close error | done | 79c6f5d |
| F3 | resumable part/sidecar dedup | done | 994b441 |
| F4 | SafeWriteOption struct-only hooks | done | 53e031d |
| F5 | decideResume pure-function extraction | done | 8e605d1 |
| F6 | fs.ModeSymlink consistency | done (folded into F1 commit) | 7612715 |

Baseline: all module tests green (2026-06-13).
Per-entry summaries: `SUMMARY-<entry>.md` in this directory.

F4's production call-site adaptation lives in the oci-image-copy repo (branch
main) as commit b939962
(`imagecopy: adapt to fsutil SafeWriteOption hook API (fsutil plan 01 entry F4)`).
