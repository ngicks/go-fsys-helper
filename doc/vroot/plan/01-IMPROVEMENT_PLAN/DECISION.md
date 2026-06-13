# vroot — Improvement Plan 01 — Decisions

The user was unavailable during this work; decisions below were made by the
supervising agent on its own judgment, with multi-agent review evidence and
(where noted) codex consulted as an advisor.

(Shared decisions D0/D1 — workspace scope and file naming — recorded in
`doc/stream/plan/01-IMPROVEMENT_PLAN/DECISION.md`.)

## D8 — No WalkDirRoot API split
Reviewer proposed a `WalkDirRoot[F, R]` escape-safe entry point. Rejected for
now: zero in-workspace WalkDir consumers means the new surface has no user;
the actual bugs (sibling-loop truncation, dead guard) are fixed and the
escape-safety bounds are documented on WalkDir/Sub/PathPrefixFs instead.
Revisit when a consumer appears.

## D9 — Rename-over and RemoveAll honor DisableOpenFileRemoval
The refcount guard becomes uniform across Remove/RemoveAll/Rename-over/Add
override (single unlink-and-close helper). This is a deliberate behavior
change: previously only Remove honored the option; the option's documented
intent (windows-like semantics) implies all unlink paths.

## D10 — Node owns mode/mtime; views own bytes/size
For bound files, handle Chmod routes to the node's metadata (under state.mu);
`view.Stat()` stays authoritative for Size only (already documented in
view.go). `memHandle.Chmod` delegates instead of writing the buffer-local mode.
This matches the synthfs-redesign direction (FileView = storage, tree = meta).

## D11 — sftpfs stays an escapable vroot.Fs
The doc fix states plainly that sftpfs is NOT symlink-confined (like osfs.Fs).
Making it a confined Root was considered out of scope: confinement over SFTP
requires server-side realpath discipline that the protocol does not guarantee.
