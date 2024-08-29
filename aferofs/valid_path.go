package aferofs

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

func toFsPath(p string) string {
	if p == "" {
		return p
	}
	p, _ = strings.CutPrefix(p, filepath.VolumeName(p))
	p = filepath.ToSlash(p)
	p = path.Clean(p)
	p, _ = strings.CutPrefix(p, "./")
	p, _ = strings.CutPrefix(p, "/")
	return p
}

// ValidPathFs is wrapper for afero.Fs that
// converts every path element into fs.FS style.
type ValidPathFs struct {
	inner afero.Fs
}

func (fsys *ValidPathFs) modifyPath(method string, oldname string, newname string) (string, string) {
	if method == "Name" {
		return oldname, newname
	}
	return toFsPath(oldname), toFsPath(newname)
}
