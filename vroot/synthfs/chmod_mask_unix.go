//go:build unix || (js && wasm) || wasip1

package synthfs

import "os"

const chmodMask = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
