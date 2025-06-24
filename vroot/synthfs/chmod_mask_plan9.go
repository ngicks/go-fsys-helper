package synthfs

import "os"

const chmodMask = os.ModePerm | os.ModeAppend | os.ModeExclusive | os.ModeTemporary
