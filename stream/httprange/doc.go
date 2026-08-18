// Package httprange translates an HTTP object into an [io.ReaderAt] by Range-Request
// so that file formats which require random access, e.g. zip, PDF, etc., can be
// read without downloading the whole object first.
package httprange
