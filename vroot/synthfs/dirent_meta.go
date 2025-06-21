package synthfs

import (
	"io/fs"
	"sync"
	"time"
)

// metadata contains common metadata fields for all directory entries
type metadata struct {
	mu       sync.RWMutex
	s        stat
	uid, gid int
}

func (m *metadata) chmod(mode fs.FileMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.mode = chmodMask & mode
}

func (m *metadata) chown(uid, gid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uid = uid
	m.gid = gid
}

func (m *metadata) chtimes(atime time.Time, mtime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !mtime.IsZero() {
		m.s.modTime = mtime
	}
	return nil
}

func (m *metadata) updateMtime(mtime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.modTime = mtime
}

func (m *metadata) rename(newname string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.name = newname
}

func (m *metadata) stat() (fs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s, nil
}

func (m *metadata) owner() (uid, gid int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uid, m.gid
}
