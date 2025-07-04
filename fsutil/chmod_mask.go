package fsutil

import (
	"io/fs"
	"os"
)

// Chmod constants are platform specific chmod mask described in [os.Chmod].
const (
	ChmodMaskPlan9   = os.ModePerm | os.ModeAppend | os.ModeExclusive | os.ModeTemporary
	ChmodMaskUnix    = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	ChmodMaskWindows = 0o200 | 0o400
)

func MaskChmodModePlan9(mode fs.FileMode) fs.FileMode {
	return mode & ChmodMaskPlan9
}

func MaskChmodModeUnix(mode fs.FileMode) fs.FileMode {
	return mode & ChmodMaskUnix
}

func MaskChmodModeWindows(mode fs.FileMode) fs.FileMode {
	writable := 0o200&mode != 0
	switch {
	case mode.IsDir() && writable:
		return mode | fs.ModePerm
	case mode.IsDir() && !writable:
		return (mode &^ fs.ModePerm) | 0o555
	case !mode.IsDir() && writable:
		return (mode &^ fs.ModePerm) | 0o666
	default: // 	case !mode.IsDir() && !writable:
		return (mode &^ fs.ModePerm) | 0o444
	}
}
