package synthfs

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

func (s stat) isSearchable() bool {
	return s.mode&0o100 > 0
}

func (s stat) isWritable() bool {
	return s.mode&0o200 > 0
}

func (s stat) isReadable() bool {
	return s.mode&0o400 > 0
}

func (s stat) IsDir() bool {
	return s.mode.IsDir()
}

func (s stat) ModTime() time.Time {
	return s.modTime
}

func (s stat) Mode() fs.FileMode {
	return s.mode
}

func (s stat) Name() string {
	return path.Base(s.name)
}

func (s stat) Size() int64 {
	return s.size
}

func (s stat) Sys() any {
	return nil
}
