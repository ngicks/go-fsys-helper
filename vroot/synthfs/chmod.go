package synthfs

import (
	"io/fs"
	"time"
)

func (r *Root) Chmod(name string, mode fs.FileMode) error {
	return r.withWriteLock(func() error {
		res, err := r.findNode(name, true, "chmod")
		if err != nil {
			return err
		}
		chmodApply(&r.st.opt, res.node.meta(), mode)
		return nil
	})
}

func (r *Root) Chown(name string, uid, gid int) error {
	return r.withWriteLock(func() error {
		res, err := r.findNode(name, true, "chown")
		if err != nil {
			return err
		}
		m := res.node.meta()
		m.uid = uid
		m.gid = gid
		return nil
	})
}

func (r *Root) Lchown(name string, uid, gid int) error {
	return r.withWriteLock(func() error {
		res, err := r.findNode(name, false, "lchown")
		if err != nil {
			return err
		}
		m := res.node.meta()
		m.uid = uid
		m.gid = gid
		return nil
	})
}

func (r *Root) Chtimes(name string, atime, mtime time.Time) error {
	return r.withWriteLock(func() error {
		res, err := r.findNode(name, true, "chtimes")
		if err != nil {
			return err
		}
		if !mtime.IsZero() {
			res.node.meta().modTime = mtime
		}
		return nil
	})
}
