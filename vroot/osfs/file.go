package osfs

import (
	"os"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.File = (*File)(nil)

// File is the file type [Fs] and [Root] return.
//
// The embedded [*os.File] promotes every [vroot.File] method; the wrapper only
// exists so osfs can attach methods the stdlib type cannot carry, namely the
// [vroot.Locker] implementation on platforms that have whole-file advisory locks.
type File struct {
	*os.File
}
