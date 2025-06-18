//go:build unix

package synthfs

import "os"

const chmodMask = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
