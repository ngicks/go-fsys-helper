package fsutil

import "io/fs"

// ChmodMask is mask for Chmod described in [os.Chmod].
const ChmodMask = ChmodMaskWindows

func MaskChmodMode(mode fs.FileMode) fs.FileMode {
	return MaskChmodModeWindows(mode)
}
