package overlayfs

import (
	"errors"
	_ "io"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var ErrTypeNotSupported = errors.New("type not suported")

// CopyPolicy defines copying behavior when file contents are needed
// to be put on top layer.
// It happens only when write to name is instructed.
// The term "write" includes not only calling Write or similar methods on [vroot.File],
// but also Chmod, Chname, Rename etc on [vroot.Fs].
//
// For simple usecase, use [CopyPolicyDotTmp],
// which only does [io.CopyBuffer] to temporary file then rename.
type CopyPolicy interface {
	// CopyTo copies files, direcoties and symlinks specified by name.
	// CopyTo returns an error that statisfies errors.Is(err, [ErrTypeNotSupported]).
	CopyTo(from Layer, to vroot.Rooted, name string) error
}
