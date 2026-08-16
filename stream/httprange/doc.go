// Package httprange exposes a remote HTTP object as an [io.ReaderAt].
//
// [New] pins a URL together with the size, the validators and the origin the
// object answered with, and every [ReaderAt.ReadAt] then issues its own
// bounded GET carrying a Range header for exactly the bytes asked for.
// Nothing is shared between reads beyond that pinned description, so any
// number of goroutines may read different parts of the object at the same
// time and no read disturbs another. When the object changes underneath, a
// read fails with [ErrObjectChanged] rather than handing back a mix of the old
// and the new bytes.
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
// [ReaderAt.Close] cancels the context New was given, which aborts the
// requests still in flight and fails every later read. The reader holds no
// other resource, so closing is cheap, always succeeds and may be done more
// than once. A reader that is never closed stops working when the caller's own
// context does.
package httprange
