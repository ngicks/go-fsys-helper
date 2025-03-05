package tarfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	curMod   GoMod
	testTars = sync.OnceValues(func() ([]string, error) {
		dir := filepath.Join("testdata", curMod.Go)
		dirents, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(dirents))
		for _, dirent := range dirents {
			if dirent.IsDir() || !strings.HasSuffix(dirent.Name(), ".tar") {
				continue
			}
			names = append(names, filepath.Join(dir, dirent.Name()))
		}
		return names, nil
	})
)

type GoMod struct {
	Module    ModPath
	Go        string
	Toolchain string
	Godebug   []Godebug
	Require   []Require
	Exclude   []Module
	Replace   []Replace
	Retract   []Retract
}

type Module struct {
	Path    string
	Version string
}

type ModPath struct {
	Path       string
	Deprecated string
}

type Godebug struct {
	Key   string
	Value string
}

type Require struct {
	Path     string
	Version  string
	Indirect bool
}

type Replace struct {
	Old Module
	New Module
}

type Retract struct {
	Low       string
	High      string
	Rationale string
}

type Tool struct {
	Path string
}

// reads go version recodred in go.mod, install sdk of that version.
// Assuming sdk is already under $PATH, it invokes go${version} download, copies contentes under $(go{version} env GOROOT)/src/archive/tar/testdata/* to
// ./testdata/go${version}
func init() {
	cmd := exec.CommandContext(context.Background(), "go", "mod", "edit", "-json")
	p, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	defer p.Close()
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	dec := json.NewDecoder(p)
	if err = dec.Decode(&curMod); err != nil {
		if cmd.Cancel != nil {
			_ = cmd.Cancel()
		} else {
			_ = cmd.Process.Kill()
		}
		panic(err)
	}
	if err := cmd.Wait(); err != nil {
		panic(err)
	}

	resourceDir := filepath.Join("testdata", curMod.Go)

	if _, err := os.Stat(resourceDir); err == nil {
		dirents, err := os.ReadDir(resourceDir)
		if err != nil {
			panic(err)
		}
		if len(dirents) > 0 {
			return
		}
	}

	if err := os.MkdirAll(resourceDir, fs.ModePerm); err != nil {
		panic(err)
	}

	if _, err := exec.Command("go", "install", fmt.Sprintf("golang.org/dl/go%s@latest", curMod.Go)).Output(); err != nil {
		printPanic(err)
	}
	if _, err := exec.Command("go"+curMod.Go, "download").Output(); err != nil {
		printPanic(err)
	}
	dir, err := exec.Command("go"+curMod.Go, "env", "GOROOT").Output()
	if err != nil {
		printPanic(err)
	}
	err = os.CopyFS(resourceDir, os.DirFS(filepath.Join(filepath.FromSlash(strings.TrimSpace(string(dir))), "src", "archive", "tar", "testdata")))
	if err != nil {
		printPanic(err)
	}
}

func printPanic(err error) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		fmt.Printf("stderr = %s\n", exitErr.Stderr)
	}
	panic(err)
}
