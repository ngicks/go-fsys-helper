package fsutil

import "os"

// ChmodMask is mask for Chmod described in [os.Chmod].
const ChmodMask = os.ModePerm | os.ModeAppend | os.ModeExclusive | os.ModeTemporary
