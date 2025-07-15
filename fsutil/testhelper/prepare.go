// Package testhelper defines some utilities to aid testing.
package testhelper

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func ExecuteLines(baseDir string, lines ...string) error {
	for _, line := range lines {
		err := ExecuteLineOs(baseDir, line)
		if err != nil {
			return err
		}
	}
	return nil
}

func ExecuteLineOs(baseDir, txt string) error {
	l := ParseLine(txt)
	if l.LineKind == "" {
		return fmt.Errorf("unknown line %q", txt)
	}
	return l.ExecuteOs(baseDir)
}

type LineKind string

const (
	LineKindMkdir     = "mkdir"
	LineKindWriteFile = "write_file"
	LineKindSymlink   = "symlink"
)

type LineDirection struct {
	LineKind   LineKind
	Permission fs.FileMode
	Path       string
	TargetPath string // for symlink target
	Content    []byte // for write file content
}

func ParseLine(txt string) LineDirection {
	switch {
	case strings.Contains(txt, "/ ") || strings.HasSuffix(txt, "/"):
		var suf string
		if strings.Contains(txt, "/ ") {
			txt, suf, _ = strings.Cut(txt, "/ ")
		} else if strings.HasSuffix(txt, "/") {
			txt = strings.TrimSuffix(txt, "/")
		}
		var perm uint64
		if suf != "" {
			perm, _ = strconv.ParseUint(suf, 0, 64)
		}
		return LineDirection{
			LineKind:   LineKindMkdir,
			Path:       txt,
			Permission: fs.FileMode(perm),
		}
	case strings.Contains(txt, ": "):
		idx := strings.Index(txt, ": ")
		path := txt[:idx]
		contentPerm := txt[idx+len(": "):]

		var perm uint64
		var content string

		// First check if content starts with a quote - if so, it's quoted content without permission
		if strings.HasPrefix(contentPerm, `"`) || strings.HasPrefix(contentPerm, "`") {
			// Try to unquote
			if unquoted, err := strconv.Unquote(contentPerm); err == nil {
				content = unquoted
			} else {
				// If unquoting fails, this is malformed
				return LineDirection{}
			}
		} else {
			// Try to parse as "permission content" format
			parts := strings.SplitN(contentPerm, " ", 2)
			if len(parts) >= 2 {
				// Check if the first part is a valid permission (numeric)
				if parsedPerm, err := strconv.ParseUint(parts[0], 0, 64); err == nil {
					perm = parsedPerm
					remainder := parts[1]
					
					// Check if the content is quoted
					if strings.HasPrefix(remainder, `"`) || strings.HasPrefix(remainder, "`") {
						// Try to unquote
						if unquoted, err := strconv.Unquote(remainder); err == nil {
							content = unquoted
						} else {
							// If unquoting fails, this is malformed
							return LineDirection{}
						}
					} else {
						// Unquoted content with spaces is not allowed when permission is specified
						if strings.Contains(remainder, " ") {
							return LineDirection{}
						}
						content = remainder
					}
				} else {
					// If first part looks like a permission attempt but fails to parse, it's malformed
					if regexp.MustCompile(`^(0[box])?[0-9a-fA-F]+$`).MatchString(parts[0]) {
						return LineDirection{}
					}
					// Otherwise, this might be content with spaces, which is not allowed unquoted
					if strings.Contains(contentPerm, " ") {
						return LineDirection{}
					}
					// Single word content without permission
					content = contentPerm
				}
			} else {
				// No space, single word content without permission
				content = contentPerm
			}
		}
		return LineDirection{
			LineKind:   LineKindWriteFile,
			Path:       path,
			Content:    []byte(content),
			Permission: fs.FileMode(perm),
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

func (l LineDirection) Equal(r LineDirection) bool {
	return l.LineKind == r.LineKind &&
		l.Permission == r.Permission &&
		l.Path == r.Path &&
		filepath.Clean(filepath.FromSlash(l.TargetPath)) == filepath.Clean(filepath.FromSlash(r.TargetPath)) &&
		bytes.Equal(l.Content, r.Content)
}

func (l LineDirection) ExecuteOs(baseDir string) error {
	perm := cmp.Or(l.Permission, fs.ModePerm) & fs.ModePerm
	baseDir = filepath.Clean(filepath.FromSlash(baseDir))
	switch l.LineKind {
	default:
		return nil
	case LineKindMkdir:
		path := filepath.Join(baseDir, filepath.FromSlash(l.Path))
		err := os.MkdirAll(path, fs.ModePerm)
		if err != nil {
			return err
		}
		return os.Chmod(path, perm)
	case LineKindWriteFile:
		path := filepath.Join(baseDir, filepath.FromSlash(l.Path))
		err := os.WriteFile(path, l.Content, fs.ModePerm)
		if err != nil {
			return err
		}
		return os.Chmod(path, perm)
	case LineKindSymlink:
		if runtime.GOOS == "plan9" {
			return nil // or just return syscall.EPLAN9?
		}
		return os.Symlink(
			filepath.FromSlash(l.TargetPath),
			filepath.Join(baseDir, filepath.FromSlash(l.Path)),
		)
	}
}

type PreapreFsysFile interface {
	io.Writer
	io.Closer
}

type PreapreFsys[File PreapreFsysFile] interface {
	Chmod(path string, mode fs.FileMode) error
	Create(path string) (File, error)
	MkdirAll(path string, perm fs.FileMode) error
	Symlink(oldname, newname string) error
}

func ExecuteLineDirection[Fsys PreapreFsys[File], File PreapreFsysFile](fsys Fsys, l LineDirection) error {
	perm := cmp.Or(l.Permission, fs.ModePerm) & fs.ModePerm
	switch l.LineKind {
	default:
		return nil
	case LineKindMkdir:
		err := fsys.MkdirAll(filepath.FromSlash(l.Path), fs.ModePerm)
		if err != nil {
			return err
		}
		return fsys.Chmod(filepath.FromSlash(l.Path), perm)
	case LineKindWriteFile:
		f, err := fsys.Create(filepath.FromSlash(l.Path))
		if err != nil {
			return err
		}
		_, err = f.Write(l.Content)
		_ = f.Close()
		if err != nil {
			return err
		}
		return fsys.Chmod(filepath.FromSlash(l.Path), perm)
	case LineKindSymlink:
		return fsys.Symlink(filepath.FromSlash(l.TargetPath), filepath.FromSlash(l.Path))
	}
}

func (l LineDirection) MustExecuteOs(baseDir string) {
	err := l.ExecuteOs(baseDir)
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

func ExecuteAllLineDirection[Fsys PreapreFsys[File], File PreapreFsysFile](fsys Fsys, seq iter.Seq[LineDirection]) error {
	for d := range seq {
		err := ExecuteLineDirection(fsys, d)
		if err != nil {
			return err
		}
	}
	return nil
}
