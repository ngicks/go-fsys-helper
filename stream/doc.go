// Package stream provides I/O stream helpers: virtual concatenation of
// ReaderAt segments (NewMultiReadAtSeekCloser), sequential-friendly ReaderAt
// over an offset-opener (NewSeqReaderAt), context-cancellable readers, and
// related primitives.
package stream
