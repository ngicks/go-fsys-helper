//go:build unix || (js && wasm) || wasip1

package fsutil

import (
	"io/fs"
)

// ChmodMask is mask for Chmod described in [os.Chmod].
const ChmodMask = ChmodMaskUnix

func maskChmodMode(mode fs.FileMode) fs.FileMode {
	return MaskChmodModeUnix(mode)
}
