package fsutil

import (
	"io/fs"
)

// ChmodMask is mask for Chmod described in [os.Chmod].
const ChmodMask = ChmodMaskPlan9

func maskChmodMode(mode fs.FileMode) fs.FileMode {
	return MaskChmodModePlan9(mode)
}
