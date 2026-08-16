# Advisory verification

## Verdict

**Request changes.**

## Blocking

- `vroot-adapter/overlayfs/file.go:317` — `mergedDir.mutate` mutates before
  checking `d.closed`. `Chmod` and `Chown` can therefore modify the filesystem
  through a closed handle instead of returning `fs.ErrClosed`.
- `vroot-adapter/overlayfs/state.go:65` — a nil lower passed to `New` is
  dereferenced at line 71, causing a panic. Only the top is validated.
- `vroot-adapter/overlayfs/datasource.go:35` — documentation recommends sharing
  one `*DataSource` between overlays. Those overlays have separate in-memory
  mask state, and closing either overlay closes the shared layer, invalidating
  the other. `vroot-adapter/overlayfs/README.md:124` repeats this unsafe
  guidance.
- `vroot-adapter/overlayfs/go.mod:8` — known release-ordering blocker.
  `GOWORK=off go test ./...` fails because the pinned published `vroot`
  predates `Locker` and `LockLevel`.

## Minor

- `vroot-adapter/overlayfs/file.go:230` — positive `SeekStart` offsets are
  silently treated as zero despite the documented rewind-only contract.
- `vroot-adapter/overlayfs/remove.go:85` — `RemoveAll` checks open handles on
  the named directory but not its descendants.
- `vroot/lock.go:20` — advisory locking does not protect against
  non-cooperating process access, so the comment overstates its guarantee.
- `vroot-adapter/overlayfs/README.md:252` — SQLite zeroes the journal header,
  not necessarily the entire journal file.

The no-`Locker` SQLite fallback was also examined. It lacks cross-instance
exclusion, but that is explicitly accepted in D22 under the sole-user
precondition, so it is not counted as a new blocker.

## Verification

Green:

- `go test ./... -count=1`: `fsutil`, `vroot`, and overlayfs
- `go test -race ./... -count=1`: `vroot` and overlayfs
- `go vet ./...`
- `golangci-lint run ./...`: zero issues
- `go mod tidy -diff`
- Linux, Darwin, and Plan 9 builds
- `git diff --check`

Windows cross-build was inconclusive because the installed Nix Go
distribution lacks Windows standard-library sources.

All five advisory review focuses ran read-only over the complete 109-path
HEAD-relative worktree scope, including untracked files. No implementation
files were changed by the review.
