package vmesh

import (
	"container/list"
	"io/fs"
	"sync"
	"time"
)

type dir struct {
	mu       sync.RWMutex
	mode     fs.FileMode
	uid, gid int
	modTime  time.Time
	// dirents and direntMap hold same objects.
	// To refer them by name, use direntMap,
	// to refer them by insertion order or something, use dirents.
	//
	// dirents is needed to prevent Readdir from returning randomly ordered result.
	dirents   *list.List
	direntMap map[string]*list.Element
}

func newDirData(mode fs.FileMode, modTime time.Time, dirents ...*dirent) *dir {
	d := &dir{
		mode:      (fs.ModeDir | mode) & (fs.ModeType | fs.ModePerm),
		modTime:   modTime,
		dirents:   list.New(),
		direntMap: make(map[string]*list.Element),
	}
	for _, dirent := range dirents {
		d.AddDirent(dirent)
	}
	return d
}

func (d *dir) lookup(name string) (ent *dirent, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ele, ok := d.direntMap[name]
	if ele != nil {
		ent = ele.Value.(*dirent)
	}
	return ent, ok
}

func (d *dir) notifyClose() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ele := d.dirents.Front(); ele != nil; ele = ele.Next() {
		_ = ele.Value.(*dirent).notifyClose()
	}
}

func (d *dir) ListFileInfo() ([]fs.FileInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	snapshot := make([]fs.FileInfo, d.dirents.Len())
	ele := d.dirents.Front()
	for i := 0; ele != nil; i++ {
		dirent := ele.Value.(*dirent)
		var err error
		snapshot[i], err = dirent.stat()
		if err != nil {
			return nil, err
		}
		ele = ele.Next()
	}
	return snapshot, nil
}

func (d *dir) ListName() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	all := make([]string, d.dirents.Len())
	ele := d.dirents.Front()
	for i := 0; ele != nil; i++ {
		dirent := ele.Value.(*dirent)
		all[i] = dirent.name
		ele = ele.Next()
	}
	return all
}

func (d *dir) Stat(path string) (stat, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return stat{
		mode:    fs.ModeDir | d.mode.Perm(),
		modTime: d.modTime,
		name:    path,
		size:    4096,
	}, nil
}

func (d *dir) Chmod(mode fs.FileMode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mode = mode & (fs.ModeType | fs.ModePerm)
}

func (d *dir) Chown(uid, gid int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.uid, d.gid = uid, gid
}

func (d *dir) Chtimes(_, mtime time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !mtime.IsZero() {
		d.modTime = mtime
	}
}

func (d *dir) AddDirent(u *dirent) (replaced *dirent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	old := d.direntMap[u.name]
	if old != nil {
		replaced = old.Value.(*dirent)
		d.dirents.Remove(old)
	}
	d.direntMap[u.name] = d.dirents.PushBack(u)
	return nil
}

func (d *dir) RemoveName(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u := d.direntMap[name]
	delete(d.direntMap, name)
	if u == nil {
		return
	}
	d.dirents.Remove(u)
}

func (d *dir) RemoveDirent(u *dirent) {
	d.RemoveName(u.name)
}

func (d *dir) Mode() fs.FileMode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.mode
}

func (d *dir) Owner() (uid, gid int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.uid, d.gid
}

func (d *dir) Times() (atime, mtime time.Time) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return time.Time{}, d.modTime
}

func (d *dir) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dirents.Len()
}
