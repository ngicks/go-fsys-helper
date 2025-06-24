package acceptancetest

import (
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var RootFsysDirections = []LineDirection{}

func init() {
	for _, l := range RootFsys {
		RootFsysDirections = append(RootFsysDirections, ParseLine(l))
	}
}

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

func MakeFsys(tempDir string, readable, writable bool) {
	for _, txt := range RootFsys {
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
	l := ParseLine(txt)
	if l.LineKind == "" {
		return fmt.Errorf("unknown line %q", txt)
	}
	return l.Execute(baseDir)
}

type LineKind string

const (
	LineKindMkdir     = "mkdir"
	LineKindWriteFile = "write_file"
	LineKindSymlink   = "symlink"
)

type LineDirection struct {
	LineKind   LineKind
	Path       string
	TargetPath string // for symlink target
	Content    []byte // for write file content
}

func ParseLine(txt string) LineDirection {
	switch {
	case strings.HasSuffix(txt, "/"):
		return LineDirection{
			LineKind: LineKindMkdir,
			Path:     txt,
		}
	case strings.Contains(txt, ": "):
		idx := strings.Index(txt, ": ")
		path := txt[:idx]
		content := txt[idx+len(": "):]
		return LineDirection{
			LineKind: LineKindWriteFile,
			Path:     path,
			Content:  []byte(content),
		}
	case strings.Contains(txt, " -> "):
		idx := strings.Index(txt, " -> ")
		path := txt[:idx]
		target := txt[idx+len(" -> "):]
		return LineDirection{
			LineKind:   LineKindSymlink,
			Path:       path,
			TargetPath: target,
		}
	}
	return LineDirection{}
}

func (l LineDirection) Execute(baseDir string) error {
	baseDir = filepath.FromSlash(filepath.Clean(baseDir))
	switch l.LineKind {
	default:
		return nil
	case LineKindMkdir:
		return os.MkdirAll(filepath.Join(baseDir, filepath.FromSlash(l.Path)), fs.ModePerm)
	case LineKindWriteFile:
		return os.WriteFile(filepath.Join(baseDir, filepath.FromSlash(l.Path)), l.Content, fs.ModePerm)
	case LineKindSymlink:
		if runtime.GOOS == "plan9" {
			return nil
		}
		return os.Symlink(filepath.FromSlash(l.TargetPath), filepath.Join(baseDir, filepath.FromSlash(l.Path)))
	}
}

func (l LineDirection) MustExecute(baseDir string) {
	err := l.Execute(baseDir)
	if err != nil {
		panic(err)
	}
}

func FilterLineDirection(fn func(l LineDirection) bool, seq iter.Seq[LineDirection]) iter.Seq[LineDirection] {
	return func(yield func(LineDirection) bool) {
		for d := range seq {
			if fn(d) && !yield(d) {
				return
			}
		}
	}
}
