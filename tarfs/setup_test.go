package tarfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	curMod   GoMod
	testTars = sync.OnceValues(func() ([]string, error) {
		testdataDir, err := stdTarTestDir(curMod.Go)
		if err != nil {
			return nil, err
		}
		dirents, err := os.ReadDir(testdataDir)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(dirents))
		for _, dirent := range dirents {
			if !dirent.Type().IsRegular() || !strings.HasSuffix(dirent.Name(), ".tar") {
				continue
			}
			names = append(names, filepath.Join(testdataDir, dirent.Name()))
		}
		return names, nil
	})
)

func stdTarTestDir(version string) (string, error) {
	goroot, err := exec.Command("go"+version, "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.FromSlash(strings.TrimSpace(string(goroot))), "src", "archive", "tar", "testdata"), nil
}

// reads go version recodred in go.mod, install sdk of that version.
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
		_, _ = io.Copy(io.Discard, p)
		_ = cmd.Wait()
		panic(err)
	}
	if err := cmd.Wait(); err != nil {
		panic(err)
	}

	resourceDir, err := stdTarTestDir(curMod.Go)
	if err == nil {
		_, err := os.Stat(resourceDir)
		if err == nil {
			dirents, err := os.ReadDir(resourceDir)
			if err != nil {
				panic(err)
			}
			if len(dirents) > 0 {
				return
			}
		}
	}

	if _, err := exec.Command("go", "install", fmt.Sprintf("golang.org/dl/go%s@latest", curMod.Go)).Output(); err != nil {
		printPanic(err)
	}
	if _, err := exec.Command("go"+curMod.Go, "download").Output(); err != nil {
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
