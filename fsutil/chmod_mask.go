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

// MaskChmodMode is one of [MaskChmodModePlan9], [MaskChmodModeUnix] or [MaskChmodModeWindows]
// depending on GOOS.
func MaskChmodMode(mode fs.FileMode) fs.FileMode {
	return maskChmodMode(mode)
}

// MaskChmodModePlan9 masks mode using [ChmodMaskPlan9].
func MaskChmodModePlan9(mode fs.FileMode) fs.FileMode {
	return mode & ChmodMaskPlan9
}

// MaskChmodModeUnix masks mode using [ChmodMaskUnix].
func MaskChmodModeUnix(mode fs.FileMode) fs.FileMode {
	return mode & ChmodMaskUnix
}

// MaskChmodModeWindows changes mode as [os.Chmod] does in windows platform.
// If given mode is a directory, returns mode with 0o777(read-write) or 0o555(read-only) permission.
// Else, return mode with 0o666(read-write) or 0o444(read-only) permission.
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
