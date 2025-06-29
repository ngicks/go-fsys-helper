//go:build unix || (js && wasm) || wasip1

package fsutil

import "os"

// ChmodMask is mask for Chmod described in [os.Chmod].
const ChmodMask = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
