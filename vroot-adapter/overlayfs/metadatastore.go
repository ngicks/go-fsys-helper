package overlayfs

import (
	"path"
	"strings"
)

// MetadataStore persists one layer's masking state: the whited-out names and
// the opaque directories. It is never on a lookup path — the overlay reads it
// once through [MetadataStore.Load] and answers every query from memory
// afterwards — but every setter is durable when it returns, so there is no
// flush step.
//
// Names are clean, slash-separated paths relative to the layer's content root;
// "" is the root itself. Implementations normalize what they are given by
// cleaning the name as if it were rooted, so "a/b", "./a/b", "a/b/", "/a/b"
// and "x/../a/b" all address one node and no ".." can walk out of the layer.
// The separator is "/" on every platform: a backslash is an ordinary character
// in a name, and a caller holding an OS path converts it first.
//
// A node carrying neither mark is forgotten, so [MetadataStore.Load] returns
// only names some setter left marked.
type MetadataStore interface {
	// Load returns every whited-out name and every opaque directory, in no
	// particular order. A name carrying both marks appears in both slices.
	// It is called once, when the owning DataSource is constructed or attached.
	Load() (whiteouts []string, opaques []string, err error)
	// SetWhiteout masks name and, with it, everything the layers below show
	// under name.
	//
	// Marks recorded strictly under name are dropped: nothing under a masked
	// name can be addressed any more, and re-creating the directory marks it
	// opaque again anyway. name's own opaque mark is dropped as well — the
	// directory that carried it is gone, and what replaces it may not be a
	// directory at all.
	SetWhiteout(name string) error
	// ClearWhiteout unmasks name, leaving its opaque mark alone. Clearing a
	// name that carries no whiteout is not an error.
	ClearWhiteout(name string) error
	// SetOpaque marks dir opaque: the layers below it stop contributing
	// children to it. Unlike [MetadataStore.SetWhiteout] it keeps the marks
	// recorded under dir, which stay addressable — dir itself is still there,
	// and clearing the opaque mark brings the layers below back into view.
	SetOpaque(dir string) error
	// ClearOpaque removes dir's opaque mark, leaving its whiteout alone.
	// Clearing a dir that carries no opaque mark is not an error.
	ClearOpaque(dir string) error
	// Close releases the store. No other method may be called afterwards.
	Close() error
}

// cleanName normalizes a name to the store's path form. Cleaning the name as
// if it were rooted is what absorbs a leading "..", so no argument can address
// a node outside the layer.
func cleanName(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}

// parentName is [path.Dir] over a cleaned name, whose root spells "" where
// path.Dir would say ".". The root is its own parent, so a query listing the
// children of "" has to exclude it.
func parentName(name string) string {
	i := strings.LastIndexByte(name, '/')
	if i < 0 {
		return ""
	}
	return name[:i]
}
