# go-fsys-helper

A collection of go modules that implement useful functions around fs.FS, io.Reader/Writer, ~~afero.Fs~~(I'm moving away from `afero`) and etc

All modules should compile on every platform except GOOS=ios and GOOS=android.
But only tested for `linux/amd64`, `linux/arch64`, `darwin/amd64`, `darwin/arch64` and `windows/amd64` (see ./.github).

## vroot

A file system abstraction library that requires capability which `*os.Root` has at least.
Totally WIP.

## vroot-adapter

Under `vroot-adapter`, vroot to/from other filesystem abstraction library adapters are hosted.
Each sub directory is separate go modules so that dependencies are totally isolated even in `go.sum`.

## tarfs

A WIP implementation of tarfs.
It receives `io.ReaderAt` that reads tar file, collects header information and returns tar as `fs.FS`.

Files aquired through this fs implements `io.ReaderAt`.

Unlike [github.com/nlepage/go-tarfs](https://github.com/nlepage/go-tarfs), this implementation handles hole.
Currently holes reads just as `0x00`. (Maybe this will change after [#22735](https://github.com/golang/go/issues/22735) is implemented.)

## fsutil

Filesystem-abstraction-library interaoperable utilities.

It should be working well with [afero](https://github.com/spf13/afero/), [go-billy](https://github.com/go-git/go-billy), [hackpadfs](https://github.com/hack-pad/hackpadfs) and of course `vroot`.

## stream

Helpers around `io.Reader` / `io.Writer`.

- `NewCancellable`: a simple wrapper that makes `io.Reader` cancellable by `context.Context`.
  - Already blocking `Read` cannot be interrupted since Go ignores `EINTR` error automatically.
  - Additinal efforts needed by yourself if you want to cancel long blocking reads.
    - Use `epoll`/`kqueue`/`poll`/`IOCP` with os pipes.
- `NewMultiReadAtSeekCloser`: virtually concatenates `io.ReaderAt`
- `NewByteRepeater` returns infinite reader that reads given byte.
  - This is mainly for creating hole reader for `tarfs`.

## FROZEN: aferofs

I'm moving away from afero.

This module is completely frozen.
