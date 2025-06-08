package vroot

import "github.com/ngicks/go-fsys-helper/vroot"

// Overlay overlays multiple layers and provides virtually concatenated view..
type Overlay struct {
	copyPolicy CopyPolicy
	layers     []Layer
}

// NewOverlay returns virtually concatenated view of layers as [vroot.Rooted].
//
// Any write operation writes data to top.
// If file is existent under any of merged layers, calling write method on it
// copies file content using copyPolicy.
// The "write" includes not only Write on files but also Chmod, Chtime, etc.
//
// If Remove or RemoveAll is called on non-existent on top layer but in layers,
// whiteout data would be updated as the path appears empty.
func NewOverlay(copyPolicy CopyPolicy, top Layer, layers ...Layer) *Overlay {
	return &Overlay{
		copyPolicy: copyPolicy,
		layers:     append([]Layer{top}, layers...),
	}
}

type Layer struct {
	meta MetadataStore
	fsys vroot.Rooted
}

type MetadataStore interface {
	RecordWhiteout(name string) error
	RemoveWhiteout(name string) error
	QueryWhiteout(name string) (has bool, err error)
}

type CopyPolicy interface {
	CopyTo(from, to Layer, name string)
}
