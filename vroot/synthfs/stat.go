package synthfs

import (
	"io/fs"
	"path"
	"time"
)

var _ fs.FileInfo = stat{}

// stat is the [fs.FileInfo] returned by synthfs for tree entries. Its zero
// value is meaningless; populate it from node metadata via [metadata.toStat].
type stat struct {
	name    string // basename within the parent directory
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (s stat) Name() string       { return path.Base(s.name) }
func (s stat) Size() int64        { return s.size }
func (s stat) Mode() fs.FileMode  { return s.mode }
func (s stat) ModTime() time.Time { return s.modTime }
func (s stat) IsDir() bool        { return s.mode.IsDir() }
func (s stat) Sys() any           { return nil }
