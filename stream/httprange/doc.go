// Package httprange exposes a remote HTTP object as an [io.ReaderAt].
//
// [New] pins a URL and puts nothing on the wire; every [ReaderAt.ReadAt] then
// issues its own bounded GET carrying a Range header for exactly the bytes
// asked for. The size of the object, the validators and the origin that later
// responses are held to are settled by the first response to arrive, a read's
// own or [ReaderAt.Probe]'s, and whatever [Config] carried is trusted until
// then and checked against that response when it comes. Validators saved from
// an earlier download are how a resumed download refuses to splice bytes of
// another object onto the ones already saved. Nothing is shared between reads
// beyond that settled description, so any number of goroutines may read
// different parts of the object at the same time and no read disturbs another.
// [ReaderAt.Metadata] hands the description back, to save for the next resume.
// When the object changes underneath, a read fails with [ErrObjectChanged]
// rather than handing back a mix of the old and the new bytes.
//
// The price is one HTTP round trip per ReadAt. Walking an object front to back
// a few kilobytes at a time is therefore painfully slow; give such a scan a
// buffer so that it turns into a handful of large requests instead. The size
// is nothing the scan needs to know: reads run before it is settled, and the
// object's own end is what ends the walk.
//
//	r, err := httprange.New(ctx, url, nil)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	br := bufio.NewReaderSize(io.NewSectionReader(r, 0, math.MaxInt64), 1<<20)
//
// A caller who does want the size, or who wants a changed object to be found
// out before a single byte is downloaded, calls [ReaderAt.Probe] first and
// reads the size off [ReaderAt.Metadata].
//
// [ReaderAt.Close] cancels a context derived from the one New was given, which
// aborts the requests still in flight and fails every later read; the caller's
// own context is left alone. The reader holds no other resource, so closing is
// cheap, always succeeds and may be done more than once. A reader that is
// never closed stops working when the caller's own context does.
package httprange
