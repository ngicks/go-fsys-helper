// Package httprange exposes a remote HTTP object as an [io.ReaderAt].
//
// [New] pins a URL together with the size of the object, and every
// [ReaderAt.ReadAt] then issues its own bounded GET carrying a Range header
// for exactly the bytes asked for. The validators and the origin that later
// responses are held to are pinned alongside the size when New probes for it,
// and by the first read that succeeds when the caller supplied the size
// instead; validators saved from an earlier download pin them ahead of either,
// which is how a resumed download refuses to splice bytes of another object
// onto the ones already saved. Nothing is shared between reads beyond that
// pinned description, so any number of goroutines may read different parts of
// the object at the same time and no read disturbs another.
// [ReaderAt.Metadata] hands that description back, to save for the next
// resume. When the object changes underneath, a read fails with
// [ErrObjectChanged] rather than handing back a mix of the old and the new
// bytes.
//
// The price is one HTTP round trip per ReadAt. Walking an object front to back
// a few kilobytes at a time is therefore painfully slow; give such a scan a
// buffer so that it turns into a handful of large requests instead:
//
//	r, err := httprange.New(ctx, url, nil)
//	if err != nil {
//		return err
//	}
//	defer r.Close()
//	br := bufio.NewReaderSize(io.NewSectionReader(r, 0, r.Size()), 1<<20)
//
// [ReaderAt.Close] cancels a context derived from the one New was given, which
// aborts the requests still in flight and fails every later read; the caller's
// own context is left alone. The reader holds no other resource, so closing is
// cheap, always succeeds and may be done more than once. A reader that is
// never closed stops working when the caller's own context does.
package httprange
