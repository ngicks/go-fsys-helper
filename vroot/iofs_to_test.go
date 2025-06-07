package vroot_test

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

var readbleFiles []string

func init() {
	for _, txt := range acceptancetest.RootFsys {
		if !strings.HasPrefix(txt, "root/readable") {
			continue
		}
		switch {
		case strings.Contains(txt, ": "):
			idx := strings.Index(txt, ": ")
			path := txt[:idx]
			readbleFiles = append(readbleFiles, strings.TrimPrefix(path, "root/readable/"))
		}
	}
}

func TestIoFsRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	fsys := vroot.ToIoFsRooted(r)
	fstest.TestFS(fsys, readbleFiles...)
}

func TestIoFsUnrooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	fsys := vroot.ToIoFsUnrooted(r)
	fstest.TestFS(fsys, readbleFiles...)
}
