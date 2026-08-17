// Package httprange exposes a remote HTTP object as an [io.ReaderAt].
//
// [New] pins a URL and puts nothing on the wire; every [ReaderAt.ReadAt] then
// issues its own bounded GET carrying a Range header for exactly the bytes
// asked for. The size of the object, the validators and the origin that later
// responses are held to are settled by the first response to arrive, a read's
// own or [ReaderAt.Probe]'s, and whatever [Config] carried is trusted until
// then and checked against that response when it comes. Validators saved from
// an earlier download are how a resumed download refuses to splice bytes of
// another object onto the ones already saved. Nothing is shared between such
// reads beyond that settled description, so any number of goroutines may read
// different parts of the object at the same time and no read disturbs another.
// [ReaderAt.Metadata] hands the description back, the total size of the object
// among it, to save for the next resume. When the object changes underneath, a
// read fails with [ErrObjectChanged] rather than handing back a mix of the old
// and the new bytes.
//
// The price is one HTTP round trip per ReadAt, which makes walking an object
// front to back a few kilobytes at a time painfully slow. There are two ways
// around it, and which one fits depends on what the caller knows when the
// reader is built. Reads that scatter, or a stretch that is not known up front,
// want a buffer over a section reader, so that the walk turns into a handful of
// large requests rather than one per buffer:
//
//	r, err := httprange.New(ctx, url, nil)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	br := bufio.NewReaderSize(io.NewSectionReader(r, 0, math.MaxInt64), 1<<20)
//
// A caller who does know up front that they will read one stretch front to
// back — copying the whole object out, resuming a download from where it
// stopped — says so with [NewRange] and has a single streaming request serve
// the whole walk:
//
//	cfg := &httprange.Config{ETag: etag} // the validator the earlier attempt saw
//	r, err := httprange.NewRange(ctx, url, saved, math.MaxInt64, cfg)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	_, err = io.Copy(dst, io.NewSectionReader(r, 0, math.MaxInt64))
//
// Building a reader of either kind puts nothing on the wire: the stream opens
// on the first read it can serve, and a read arriving anywhere but where that
// stream stands is a bounded request of its own, so a declaration the caller
// does not keep to costs round trips and nothing else.
//
// The size is nothing either walk needs to know: reads run before it is
// settled, and the object's own end is what ends the walk. The first request
// settles what was open and checks what was handed in at once, so a server that
// does not honour Range, a status the reader cannot work with and an object
// other than the one [Config] described all surface at that first read. A
// caller who wants the size, or wants those found out before a single byte is
// downloaded, calls [ReaderAt.Probe] first and reads the size off
// [ReaderAt.Metadata].
//
// [ReaderAt.Close] cancels a context derived from the one New was given, which
// aborts the requests still in flight, fails every later read and hands back
// the connection a [NewRange] reader's stream was holding; the caller's own
// context is left alone. There is nothing else to release, so closing is cheap,
// always succeeds and may be done more than once. A reader that is never closed
// stops working when the caller's own context does.
package httprange
