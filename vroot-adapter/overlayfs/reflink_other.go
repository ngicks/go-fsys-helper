//go:build !linux

package overlayfs

import "github.com/ngicks/go-fsys-helper/vroot"

// reflinkFile reports that no clone was made. Only Linux's FICLONE is wired up
// here — the other platforms' equivalents (APFS clonefile, ReFS block cloning)
// are reached through no interface vroot exposes.
func reflinkFile(dst, src vroot.File) bool {
	return false
}
