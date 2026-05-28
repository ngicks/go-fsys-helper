// Package memfs is a thin convenience wrapper over [synthfs]. It exposes the
// default-configured synthfs.Root as an in-memory filesystem rooted at name.
//
// Use synthfs directly when you need to customize the allocator, umask, clock,
// or chmod mask — memfs is just `synthfs.NewRoot(name, nil)`.
package memfs

import "github.com/ngicks/go-fsys-helper/vroot/synthfs"

// New returns a fresh, empty in-memory [synthfs.Root]. name is reported by
// the Root's Name method. The returned Root uses defaults: real wall clock,
// in-memory allocator, 0o022 umask, simple chmod mask, no sharing-violation
// emulation.
func New(name string) *synthfs.Root {
	return synthfs.NewRoot(name, nil)
}
