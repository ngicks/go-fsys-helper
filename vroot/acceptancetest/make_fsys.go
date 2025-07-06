package acceptancetest

import (
	"strings"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
)

var RootFsysDirections = []testhelper.LineDirection{}

func init() {
	for _, l := range RootFsys {
		RootFsysDirections = append(RootFsysDirections, testhelper.ParseLine(l))
	}
}

// RootFsysReadableFiles is a list of paths that should exist under "root/readable".
// All paths are "root/readable" prefix trimmed.
var RootFsysReadableFiles []string

func init() {
	for _, txt := range RootFsys {
		if !strings.HasPrefix(txt, "root/readable") {
			continue
		}
		switch {
		case strings.Contains(txt, ": "):
			idx := strings.Index(txt, ": ")
			path := txt[:idx]
			RootFsysReadableFiles = append(RootFsysReadableFiles, strings.TrimPrefix(path, "root/readable/"))
		}
	}
}

func MakeOsFsys(tempDir string, readable, writable bool) {
	for _, txt := range RootFsys {
		if !readable && strings.HasPrefix(txt, "root/readable") {
			continue
		}
		if !writable && strings.HasPrefix(txt, "root/writable") {
			continue
		}

		if err := testhelper.ExecuteLineOs(tempDir, txt); err != nil {
			panic(err)
		}
	}
}

