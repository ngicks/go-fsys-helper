// Package httprange exposes a remote HTTP object as an [io.ReaderAt], so that
// a format meant to be read in pieces — a zip archive found through the
// directory at its tail, a PDF found through its cross-reference table,
// anything at all that reads through ReadAt — can be read where it lies, out
// of an S3-compatible object store or any HTTP server honouring Range
// requests, without downloading the whole object first.
//
// How it reads, and what that costs:
//
//   - Every [ReaderAt.ReadAt] issues one bounded GET carrying a Range header
//     for exactly the bytes asked for, which is one HTTP round trip per call.
//     Ask for large pieces, or put a caching [io.ReaderAt] in front of the
//     reader, so that a walk over the object does not spend a round trip every
//     few kilobytes. A bufio.Reader is not that: it hands back a sequential
//     reader and gives up the reading at an offset this package exists for.
//   - [NewRange] is for the caller who knows up front that they will read one
//     stretch mostly front to back — copying the object out, resuming a
//     download. One streaming request serves the reads that arrive in order,
//     so such a walk costs a single round trip however many reads it takes. A
//     read landing anywhere else is answered by a bounded request of its own;
//     the stream closes there for good and every read from then on is bounded
//     again, so a declaration the caller does not keep to costs round trips
//     and nothing else.
//   - Reads may run concurrently, and a bounded read shares nothing with the
//     others beyond what the reader knows about the object, so any number of
//     goroutines may read different parts of it at once and none disturbs
//     another.
//
// What the reader knows about the object, and when it finds out:
//
//   - Building a reader puts nothing on the wire. What [Config] carries in
//     PriorKnowledge — a size, validators saved from an earlier download, any
//     subset of them, nothing at all — is what the reader starts out believing,
//     trusted until a request verifies it.
//   - The size, the validators and the origin that later responses are held to
//     settle one property at a time, each by the first response the reader
//     accepts that states it. Reads run before any of that is settled; the
//     object's own end is what ends a walk.
//   - A response describing another object fails with [ErrObjectChanged]
//     rather than handing back a mix of the old bytes and the new, which is
//     how a resumed download refuses to splice one object onto another. A
//     server answering a range with the whole entity fails with
//     [ErrRangeIgnored], and a status the reader cannot work with comes back
//     as a [*StatusCodeError].
//   - [ReaderAt.Probe] puts those failures, and the size, ahead of the first
//     byte for a caller who would rather not meet them mid-walk. Skipping it
//     costs nothing: the same check runs inside whichever request the reader
//     makes first, over that request's own response.
//   - [ReaderAt.Metadata] hands back what the reader has pinned, the total size
//     among it, to save for the next resume, along with the headers of the
//     first response it accepted — where a Content-Disposition or a
//     Content-Type is read off, no HEAD request of one's own needed. It may be
//     asked at any time, including while a walk is running.
//   - [ReaderAt.Close] cancels a context derived from the one the reader was
//     built with, which aborts the requests still in flight, fails every later
//     read and hands back the connection a [NewRange] reader's stream was
//     holding; the caller's own context is left alone. There is nothing else to
//     release, so closing is cheap, always succeeds and may be done more than
//     once. A reader that is never closed stops working when the caller's own
//     context does.
//
// Reading an archive out of an object store, which is what the size is wanted
// up front for:
//
//	r, err := httprange.New(ctx, url, nil)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	if err := r.Probe(ctx); err != nil { // settles the size before the first read
//		return err
//	}
//	meta, _ := r.Metadata()
//	zr, err := zip.NewReader(r, meta.Size)
//
// Walking one stretch front to back — here a download resumed from where an
// earlier attempt stopped, which is the same call with an offset of zero and
// nothing known:
//
//	cfg := &httprange.Config{PriorKnowledge: saved} // what the earlier attempt saw
//	r, err := httprange.NewRange(ctx, url, int64(len(local)), math.MaxInt64, cfg)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	_, err = io.Copy(dst, io.NewSectionReader(r, 0, math.MaxInt64))
package httprange
