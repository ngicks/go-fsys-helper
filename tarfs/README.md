# tarfs

tarfs receives [io.ReaderAt](https://pkg.go.dev/io#ReaderAt) that reads a tar file.
It exposes files and directories within the archive as [fs.FS](https://pkg.go.dev/io/fs#FS).

- `fs.File` implements `io.Seeker`, `io.ReaderAt`.
- Sparse files are handled but all holes are just read only as `0x00`.
- Symlinks and hardlinks are handled
