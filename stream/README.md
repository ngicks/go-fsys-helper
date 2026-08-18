# stream

## stream helpers

Package stream provides helpers for reading from and writing to stream.

### httprange

Package httprange exposes a remote HTTP URL as an `io.ReaderAt` backed by
bounded `Range` requests. Each `ReadAt` is a self-contained request, safe
for concurrent use, so the reader works directly with `zip.NewReader`,
tarfs, and `stream.NewMultiReadAtSeekCloser` segments. Mid-session object
changes and servers that ignore `Range` fail with explicit errors instead
of corrupting reads. Each read costs one round trip, so read in large
pieces, or put a caching `io.ReaderAt` in front of the reader, to cut them
down; wrapping in a `bufio.Reader` is not the way, since that hands back a
sequential reader and gives up the random access the package is for.

- `NewRange(ctx, url, off, n, cfg)` is for a stretch declared up front and
  read front to back: one streaming request serves the reads arriving in
  order, so a whole-object copy costs a single round trip. Offsets follow
  `io.SectionReader` over `[off, off+n)`. A read arriving anywhere else
  ends the stream for good and every read from then on is bounded again.
- `ReaderAt.Probe(ctx)` puts the failures — a changed object, a server
  ignoring `Range`, a bad status — ahead of the first byte, at the cost of
  one single-byte request; skipping it costs nothing, as the first read
  runs the same check over its own response.
- `ReaderAt.Metadata()` reports the size and the validators to save; the
  same snapshot goes back through `Config.PriorKnowledge` to resume a
  download later, and any subset of it may be filled in by hand. It also
  carries the headers of the first response the reader accepted, which is
  where `Content-Disposition`, `Content-Type` and vendor metadata are read
  off without a `HEAD` request of one's own.

## Dependencies

Nothing other than std.
