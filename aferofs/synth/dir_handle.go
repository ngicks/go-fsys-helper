package synth

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"sync"

	"github.com/ngicks/go-fsys-helper/aferofs/internal/errdef"
	"github.com/spf13/afero"
)

var _ afero.File = (*dirHandle)(nil)

// dirHandle represents an open fd for directory.
type dirHandle struct {
	mu   sync.RWMutex
	dir  *dir
	name string
	off  int64
	// This field is used to mimic Go's Readdir behavior.
	snapshot []fs.FileInfo
}

func (d *dirHandle) Close() error {
	return nil
}

func (d *dirHandle) Name() string {
	return d.name
}

func (d *dirHandle) Read(p []byte) (n int, err error) {
	return 0, errdef.ReadIsDir(d.name)
}

func (d *dirHandle) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, errdef.ReadAtIsDir(d.name)
}

func (d *dirHandle) Readdir(count int) ([]fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.snapshot == nil {
		// mimicking Go's behavior of readdir.
		// On unix, it uses getdents64. It reads dents using 8KiB buffer.
		// I'm not sure about details, but it does not notice new dir entries are added or removed.
		// Taking snapshot then keeping it until Seek is called somewhat sounds reasonable to me.
		snapshot, err := d.dir.ListFileInfo()
		if err != nil {
			return []fs.FileInfo{}, err
		}
		d.snapshot = snapshot
	}
	if count <= 0 || count >= len(d.snapshot[d.off:]) {
		var err error
		if len(d.snapshot[d.off:]) == 0 && count > 0 {
			err = io.EOF
		}
		snapshots := d.snapshot[d.off:]
		d.off = int64(len(d.snapshot))
		return snapshots, err
	}
	ret := d.snapshot[d.off : int(d.off)+count]
	d.off += int64(count)
	return ret, nil
}

func (d *dirHandle) Readdirnames(n int) ([]string, error) {
	fi, err := d.Readdir(n)
	if err != nil {
		return []string{}, err
	}
	out := make([]string, len(fi))
	for i, fi := range fi {
		out[i] = fi.Name()
	}
	return out, nil
}

func (d *dirHandle) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// reset anyway
	d.snapshot = nil
	d.off = 0

	switch whence {
	default:
		return 0, errdef.SeekInval(d.name, fmt.Sprintf("unknown whence: %d", whence))
	case io.SeekStart:
		if offset < 0 {
			return 0, errdef.SeekInval(d.name, "negative offset")
		}
	case io.SeekCurrent, io.SeekEnd:
		if offset > 0 {
			return 0, errdef.SeekInval(d.name, "positive offset")
		}
	}
	return 0, nil
}

// Stat implements afero.File.
func (d *dirHandle) Stat() (fs.FileInfo, error) {
	return d.dir.Stat(path.Base(d.name))
}

// Sync implements afero.File.
func (d *dirHandle) Sync() error {
	return nil
}

// Truncate implements afero.File.
func (d *dirHandle) Truncate(size int64) error {
	return errdef.TruncateInvalid(d.name)
}

// Write implements afero.File.
func (d *dirHandle) Write(p []byte) (n int, err error) {
	return 0, errdef.WriteBadf(d.name)
}

// WriteAt implements afero.File.
func (d *dirHandle) WriteAt(_ []byte, _ int64) (n int, err error) {
	return 0, errdef.WriteAtBadf(d.name)
}

// WriteString implements afero.File.
func (d *dirHandle) WriteString(_ string) (ret int, err error) {
	return 0, errdef.WriteAtBadf(d.name)
}
