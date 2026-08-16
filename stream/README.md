# stream

## stream helpers

Package stream provides helpers for reading from and writing to stream.

### httprange

Package httprange exposes a remote HTTP URL as an `io.ReaderAt` backed by
bounded `Range` requests. Each `ReadAt` is a self-contained request, safe
for concurrent use, so the reader works directly with `zip.NewReader`,
tarfs, and `stream.NewMultiReadAtSeekCloser` segments. Mid-session object
changes and servers that ignore `Range` fail with explicit errors instead
of corrupting reads. For sequential scans, wrap with
`io.NewSectionReader` + `bufio.Reader` to cut round-trips.

## Dependencies

Nothing other than std.
