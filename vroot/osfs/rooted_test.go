package vroot

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func makeFsys(tempDir string, readable, writable bool) {
	for _, txt := range acceptancetest.RootFsys {
		if !readable && strings.HasPrefix(txt, "root/readable") {
			continue
		}
		if !writable && strings.HasPrefix(txt, "root/writable") {
			continue
		}

		switch {
		case strings.HasSuffix(txt, "/"):
			err := os.Mkdir(filepath.Join(tempDir, filepath.FromSlash(txt)), fs.ModePerm)
			if err != nil {
				panic(err)
			}
		case strings.Contains(txt, ": "):
			idx := strings.Index(txt, ": ")
			path := txt[:idx]
			content := txt[idx+len(": "):]
			err := os.WriteFile(filepath.Join(tempDir, path), []byte(content), fs.ModePerm)
			if err != nil {
				panic(err)
			}
		case strings.Contains(txt, " -> "):
			idx := strings.Index(txt, " -> ")
			path := txt[:idx]
			target := txt[idx+len(" -> "):]
			err := os.Symlink(target, path)
			if err != nil {
				panic(err)
			}
		}
	}
}

func TestRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	makeFsys(tempDir, false, true)
	r, err := NewRooted(filepath.Join(tempDir, "root", "writable"))
	if err != nil {
		panic(err)
	}
	acceptancetest.RootedReadWrite(t, r)
}
