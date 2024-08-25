package vmesh

import (
	"io/fs"
	"path"
	"time"
)

var _ fs.FileInfo = stat{}

type stat struct {
	mode    fs.FileMode
	modTime time.Time
	name    string
	size    int64
}

// IsDir implements fs.FileInfo.
func (s stat) IsDir() bool {
	return s.mode.IsDir()
}

// ModTime implements fs.FileInfo.
func (s stat) ModTime() time.Time {
	return s.modTime
}

// Mode implements fs.FileInfo.
func (s stat) Mode() fs.FileMode {
	return s.mode
}

// Name implements fs.FileInfo.
func (s stat) Name() string {
	return path.Base(s.name)
}

// Size implements fs.FileInfo.
func (s stat) Size() int64 {
	return s.size
}

// Sys implements fs.FileInfo.
func (s stat) Sys() any {
	return nil
}
