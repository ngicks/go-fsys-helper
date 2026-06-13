# SUMMARY — V6: sftpfs error-convention conformance + doc fix

## Package doc fix (D11)

`vroot-adapter/sftpfs/sftpfs.go` package doc previously claimed
`vroot.Unrooted` — a type that does not exist — and the wording invited a false
confinement assumption. Replaced with: SftpFs implements [vroot.Fs] and is
escapable, NOT symlink-confined (like osfs.Fs); resolvePath blocks a ".." path
*argument* lexically, but symlink targets are not confined (the remote's job).
Confinement over SFTP would require server-side realpath discipline the protocol
does not guarantee — out of scope (D11). Also documents the error convention.

## Error-convention conformance

Every method previously returned the RAW sftp client error from at least some
paths, bypassing the documented `*fs.PathError` / `*os.LinkError` convention.
Notably `MkdirAll` returned the raw client error, so `errors.Is(err,
fs.ErrExist)` failed even though `Mkdir` mapped it — and oci-image-copy relies on
MkdirAll idempotency (remote.go, push.go, fsocidirs.go).

Now every client-call result is wrapped:
- path ops → `fsutil.WrapPathErr(op, name, mapSftpErr(err))`:
  Chmod, Chown, Chtimes, Lchown, Lstat, Mkdir, MkdirAll, OpenFile, ReadLink,
  Remove, Stat, ReadDir.
- link ops → `fsutil.WrapLinkErr(op, old, new, mapSftpErr(err))`:
  Link, Rename (both posix and plain), Symlink.

`WrapPathErr`/`WrapLinkErr` overwrite the error's Op/Path with the caller's
relative name, so the absolute Base never leaks into the surfaced path fields.
(The ReadLink-on-base EINVAL path was also switched from `abs` to `name` to stop
leaking the base.)

### mapSftpErr extended

`mapSftpErr` recovered only EEXIST before. SFTP v3 collapses several errnos to
SSH_FX_FAILURE, preserving only the message text, so the same loss affects EINVAL
and ENOTEMPTY. Extended mapSftpErr to also rejoin `syscall.EINVAL` ("invalid
argument") and `syscall.ENOTEMPTY` ("directory not empty") from the message. This
was required because the plan-01 V2 acceptance vector (directory-into-own-subtree
→ EINVAL) now reaches sftpfs through the shared suite: the server returned EINVAL
but pkg/sftp surfaced it as SSH_FX_FAILURE, so `errors.Is(syscall.EINVAL)` failed
until this mapping was added. The original *sftp.StatusError is preserved (joined)
so its FxCode stays observable.

## Tests

New `vroot-adapter/sftpfs/sftpfs_test.go` `TestErrorConvention`:
- MkdirAll idempotency (a second MkdirAll on the existing tree succeeds).
- Mkdir over an existing dir → `errors.Is(fs.ErrExist)` and a `*fs.PathError`
  whose Path echoes the caller name.
- Stat/Open of a missing path → `errors.Is(fs.ErrNotExist)` and `*fs.PathError`.
- Rename/Link of a missing source → `*os.LinkError` with Old/New echoing the
  caller names and no absolute-base leak.

The existing acceptance suite (`TestFs`, posixRename={false,true}) — including the
plan-01 V2 EINVAL subtree vector now reaching sftpfs — passes.

## Test evidence

```
cd go-fsys-helper/vroot-adapter/sftpfs
go build ./...                              # ok
go vet ./...                                # ok
go test ./...                               # ok (TestFs + TestErrorConvention)
go test ./... -run TestErrorConvention -v   # all PASS
```

oci-image-copy consumer:
```
cd oci-image-copy
go build ./...        # ok
go test ./pkg/...     # ok
```

## Implementation bugs found

- MkdirAll (and Lchown/Link/Rename/Stat/ReadLink/ReadDir/Symlink) returned raw
  client errors, breaking `errors.Is(fs.ErrExist)` / the *fs.PathError /
  *os.LinkError convention. Fixed.
- mapSftpErr did not recover EINVAL/ENOTEMPTY from SSH_FX_FAILURE. Fixed.
- Package doc referenced the nonexistent vroot.Unrooted. Fixed.

## Deviation from plan

In scope. The mapSftpErr EINVAL/ENOTEMPTY extension goes slightly beyond the
plan's EEXIST-focused wording, but is the same class of fix and was required to
keep the shared acceptance suite (V2 vector) green against sftpfs.

## Files changed

- `vroot-adapter/sftpfs/sftpfs.go` (doc + wrap every client call + mapSftpErr)
- `vroot-adapter/sftpfs/sftpfs_test.go` (TestErrorConvention)
