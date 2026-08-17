# stream

## stream helpers

Package stream provides helpers for reading from and writing to stream.

### httprange

Package httprange exposes a remote HTTP URL as an `io.ReaderAt` backed by
bounded `Range` requests. Each `ReadAt` is a self-contained request, safe
for concurrent use, so the reader works directly with `zip.NewReader`,
tarfs, and `stream.NewMultiReadAtSeekCloser` segments. Mid-session object
changes and servers that ignore `Range` fail with explicit errors instead
of corrupting reads. For scattered reads, or a stretch not known up front,
wrap with `io.NewSectionReader` + `bufio.Reader` to cut round-trips.

- `NewRange(ctx, url, off, n, cfg)` is for a stretch declared up front and
  read front to back: one streaming request serves the reads arriving in
  order, so a whole-object copy costs a single round trip. Offsets follow
  `io.SectionReader` over `[off, off+n)`. A read arriving anywhere else
  ends the stream for good and every read from then on is bounded again.
- `ReaderAt.Probe(ctx)` puts the failures — a changed object, a server
  ignoring `Range`, a bad status — ahead of the first byte, at the cost of
  one single-byte request; skipping it costs nothing, as the first read
  runs the same check over its own response.
- `ReaderAt.Metadata()` reports the size and the validators to save, which
  are what `Config` takes back to resume a download later.

## Dependencies

Nothing other than std.
