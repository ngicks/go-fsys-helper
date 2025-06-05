package prepare

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func MakeFsys(tempDir string, readable, writable bool) {
	for _, txt := range acceptancetest.RootFsys {
		if !readable && strings.HasPrefix(txt, "root/readable") {
			continue
		}
		if !writable && strings.HasPrefix(txt, "root/writable") {
			continue
		}

		if err := ExecuteLine(tempDir, txt); err != nil {
			panic(err)
		}
	}
}

func ExecuteLines(baseDir string, lines ...string) error {
	for _, line := range lines {
		err := ExecuteLine(baseDir, line)
		if err != nil {
			return err
		}
	}
	return nil
}

func ExecuteLine(baseDir, txt string) error {
	switch {
	case strings.HasSuffix(txt, "/"):
		err := os.Mkdir(filepath.Join(baseDir, filepath.FromSlash(txt)), fs.ModePerm)
		return err
	case strings.Contains(txt, ": "):
		idx := strings.Index(txt, ": ")
		path := txt[:idx]
		content := txt[idx+len(": "):]
		err := os.WriteFile(filepath.Join(baseDir, path), []byte(content), fs.ModePerm)
		return err
	case strings.Contains(txt, " -> "):
		idx := strings.Index(txt, " -> ")
		path := txt[:idx]
		target := txt[idx+len(" -> "):]
		err := os.Symlink(target, filepath.Join(baseDir, path))
		return err
	}
	return fmt.Errorf("unknown line %q", txt)
}
