# fsutil — Improvement Plan 01 — Decisions

The user was unavailable during this work; decisions below were made by the
supervising agent on its own judgment, with multi-agent review evidence and
(where noted) codex consulted as an advisor.

(Shared decisions D0/D1 — workspace scope and file naming — recorded in
`doc/stream/plan/01-IMPROVEMENT_PLAN/DECISION.md`.)

## D5 — Keep ResolvePath / SyncHook / ChmodMask exported
Reviewer proposed unexporting symbols with no in-workspace consumers. Rejected:
this is a published module (consumed at pinned versions by tarfs etc.); the
symbols are coherent, tested utilities; removal is churn with no consumer
benefit. Only the cosmetic `os.ModeSymlink` → `fs.ModeSymlink` consistency fix
is taken (F6).

## D6 — SafeWriteOption hooks: struct-only (breaking)
Positional `preHooks, postHooks` params removed from Write/Copy/CopyFs; hooks
live only on the struct, matching ResumableCopyOption's single-source design.
Breaking, but the only production caller passes `nil, nil`, all consumers are
in-workspace, and the dual-source execution ordering was genuinely confusing.

## D7 — WrapPathErr/WrapLinkErr: copy-on-write
The field-merge behavior (non-zero op/path override) is kept for compatibility,
but applied to a fresh copy so the caller's error value is never mutated.
Alternative (always allocate a wrapping PathError around the PathError) was
rejected: double-wrapping changes rendered messages at 40+ vroot call sites.

## D8 — Pull resume-trust extraction preserves two-phase open
`decideResume` is a pure function, but Pull keeps its existing
open-at-guessed-offset → reopen-at-0-if-untrusted sequencing so no extra source
round-trip is introduced.
