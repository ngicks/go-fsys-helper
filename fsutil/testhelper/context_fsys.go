package testhelper

import (
	"io/fs"
	"time"
)

func (c *C[T, F, Fs]) ChmodE(name string, mode fs.FileMode) error {
	c.t.Helper()
	c.record("Chmod(%q, %s)", name, mode)
	err := c.fsys.Chmod(name, mode)
	if err == nil {
		c.ReportFailf("Chmod(%q, %s) succeeded, want error", name, mode)
	}
	return err
}

func (c *C[T, F, Fs]) Chmod(name string, mode fs.FileMode) {
	c.t.Helper()
	c.record("Chmod(%q, %s)", name, mode)
	if err := c.fsys.Chmod(name, mode); err != nil {
		c.ReportFailf("Chmod(%q, %s): %v", name, mode, err)
	}
}

func (c *C[T, F, Fs]) ChownE(name string, uid int, gid int) error {
	c.t.Helper()
	c.record("Chown(%q, %d, %d)", name, uid, gid)
	err := c.fsys.Chown(name, uid, gid)
	if err == nil {
		c.ReportFailf("Chown(%q, %d, %d) succeeded, want error", name, uid, gid)
	}
	return err
}

func (c *C[T, F, Fs]) Chown(name string, uid int, gid int) {
	c.t.Helper()
	c.record("Chown(%q, %d, %d)", name, uid, gid)
	if err := c.fsys.Chown(name, uid, gid); err != nil {
		c.ReportFailf("Chown(%q, %d, %d): %v", name, uid, gid, err)
	}
}

func (c *C[T, F, Fs]) ChtimesE(name string, atime time.Time, mtime time.Time) error {
	c.t.Helper()
	c.record("Chtimes(%q, %s, %s)", name, atime, mtime)
	err := c.fsys.Chtimes(name, atime, mtime)
	if err == nil {
		c.ReportFailf("Chtimes(%q, %s, %s) succeeded, want error", name, atime, mtime)
	}
	return err
}

func (c *C[T, F, Fs]) Chtimes(name string, atime time.Time, mtime time.Time) {
	c.t.Helper()
	c.record("Chtimes(%q, %s, %s)", name, atime, mtime)
	if err := c.fsys.Chtimes(name, atime, mtime); err != nil {
		c.ReportFailf("Chtimes(%q, %s, %s): %v", name, atime, mtime, err)
	}
}

func (c *C[T, F, Fs]) CreateE(name string) (F, error) {
	c.t.Helper()
	c.record("Create(%q)", name)
	f, err := c.fsys.Create(name)
	if err == nil {
		_ = f.Close()
		c.ReportFailf("Create(%q) succeeded, want error", name)
	}
	return f, err
}

func (c *C[T, F, Fs]) Create(name string) F {
	c.t.Helper()
	c.record("Create(%q)", name)
	f, err := c.fsys.Create(name)
	if err != nil {
		c.ReportFailf("Create(%q): %v", name, err)
	}
	return f
}

func (c *C[T, F, Fs]) LchownE(name string, uid int, gid int) error {
	c.t.Helper()
	c.record("Lchown(%q, %d, %d)", name, uid, gid)
	err := c.fsys.Lchown(name, uid, gid)
	if err == nil {
		c.ReportFailf("Lchown(%q, %d, %d) succeeded, want error", name, uid, gid)
	}
	return err
}

func (c *C[T, F, Fs]) Lchown(name string, uid int, gid int) {
	c.t.Helper()
	c.record("Lchown(%q, %d, %d)", name, uid, gid)
	if err := c.fsys.Lchown(name, uid, gid); err != nil {
		c.ReportFailf("Lchown(%q, %d, %d): %v", name, uid, gid, err)
	}
}

func (c *C[T, F, Fs]) LinkE(oldname string, newname string) error {
	c.t.Helper()
	c.record("Link(%q, %q)", oldname, newname)
	err := c.fsys.Link(oldname, newname)
	if err == nil {
		c.ReportFailf("Link(%q, %q) succeeded, want error", oldname, newname)
	}
	return err
}

func (c *C[T, F, Fs]) Link(oldname string, newname string) {
	c.t.Helper()
	c.record("Link(%q, %q)", oldname, newname)
	if err := c.fsys.Link(oldname, newname); err != nil {
		c.ReportFailf("Link(%q, %q): %v", oldname, newname, err)
	}
}

func (c *C[T, F, Fs]) LstatE(name string) (fs.FileInfo, error) {
	c.t.Helper()
	c.record("Lstat(%q)", name)
	info, err := c.fsys.Lstat(name)
	if err == nil {
		c.ReportFailf("Lstat(%q) succeeded, want error", name)
	}
	return info, err
}

func (c *C[T, F, Fs]) Lstat(name string) fs.FileInfo {
	c.t.Helper()
	c.record("Lstat(%q)", name)
	info, err := c.fsys.Lstat(name)
	if err != nil {
		c.ReportFailf("Lstat(%q): %v", name, err)
	}
	return info
}

func (c *C[T, F, Fs]) MkdirE(name string, perm fs.FileMode) error {
	c.t.Helper()
	c.record("Mkdir(%q, %s)", name, perm)
	err := c.fsys.Mkdir(name, perm)
	if err == nil {
		c.ReportFailf("Mkdir(%q, %s) succeeded, want error", name, perm)
	}
	return err
}

func (c *C[T, F, Fs]) Mkdir(name string, perm fs.FileMode) {
	c.t.Helper()
	c.record("Mkdir(%q, %s)", name, perm)
	if err := c.fsys.Mkdir(name, perm); err != nil {
		c.ReportFailf("Mkdir(%q, %s): %v", name, perm, err)
	}
}

func (c *C[T, F, Fs]) MkdirAllE(name string, perm fs.FileMode) error {
	c.t.Helper()
	c.record("MkdirAll(%q, %s)", name, perm)
	err := c.fsys.MkdirAll(name, perm)
	if err == nil {
		c.ReportFailf("MkdirAll(%q, %s) succeeded, want error", name, perm)
	}
	return err
}

func (c *C[T, F, Fs]) MkdirAll(name string, perm fs.FileMode) {
	c.t.Helper()
	c.record("MkdirAll(%q, %s)", name, perm)
	if err := c.fsys.MkdirAll(name, perm); err != nil {
		c.ReportFailf("MkdirAll(%q, %s): %v", name, perm, err)
	}
}

func (c *C[T, F, Fs]) OpenE(name string) (F, error) {
	c.t.Helper()
	c.record("Open(%q)", name)
	f, err := c.fsys.Open(name)
	if err == nil {
		_ = f.Close()
		c.ReportFailf("Open(%q) succeeded, want error", name)
	}
	return f, err
}

func (c *C[T, F, Fs]) Open(name string) F {
	c.t.Helper()
	c.record("Open(%q)", name)
	f, err := c.fsys.Open(name)
	if err != nil {
		c.ReportFailf("Open(%q): %v", name, err)
	}
	return f
}

func (c *C[T, F, Fs]) OpenFileE(name string, flag int, perm fs.FileMode) (F, error) {
	c.t.Helper()
	c.record("OpenFile(%q, %d, %s)", name, flag, perm)
	f, err := c.fsys.OpenFile(name, flag, perm)
	if err == nil {
		_ = f.Close()
		c.ReportFailf("OpenFile(%q, %d, %s) succeeded, want error", name, flag, perm)
	}
	return f, err
}

func (c *C[T, F, Fs]) OpenFile(name string, flag int, perm fs.FileMode) F {
	c.t.Helper()
	c.record("OpenFile(%q, %d, %s)", name, flag, perm)
	f, err := c.fsys.OpenFile(name, flag, perm)
	if err != nil {
		c.ReportFailf("OpenFile(%q, %d, %s): %v", name, flag, perm, err)
	}
	return f
}

func (c *C[T, F, Fs]) ReadLinkE(name string) (string, error) {
	c.t.Helper()
	c.record("ReadLink(%q)", name)
	target, err := c.fsys.ReadLink(name)
	if err == nil {
		c.ReportFailf("ReadLink(%q) succeeded, want error", name)
	}
	return target, err
}

func (c *C[T, F, Fs]) ReadLink(name string) string {
	c.t.Helper()
	c.record("ReadLink(%q)", name)
	target, err := c.fsys.ReadLink(name)
	if err != nil {
		c.ReportFailf("ReadLink(%q): %v", name, err)
	}
	return target
}

func (c *C[T, F, Fs]) RemoveE(name string) error {
	c.t.Helper()
	c.record("Remove(%q)", name)
	err := c.fsys.Remove(name)
	if err == nil {
		c.ReportFailf("Remove(%q) succeeded, want error", name)
	}
	return err
}

func (c *C[T, F, Fs]) Remove(name string) {
	c.t.Helper()
	c.record("Remove(%q)", name)
	if err := c.fsys.Remove(name); err != nil {
		c.ReportFailf("Remove(%q): %v", name, err)
	}
}

func (c *C[T, F, Fs]) RemoveAllE(name string) error {
	c.t.Helper()
	c.record("RemoveAll(%q)", name)
	err := c.fsys.RemoveAll(name)
	if err == nil {
		c.ReportFailf("RemoveAll(%q) succeeded, want error", name)
	}
	return err
}

func (c *C[T, F, Fs]) RemoveAll(name string) {
	c.t.Helper()
	c.record("RemoveAll(%q)", name)
	if err := c.fsys.RemoveAll(name); err != nil {
		c.ReportFailf("RemoveAll(%q): %v", name, err)
	}
}

func (c *C[T, F, Fs]) RenameE(oldname string, newname string) error {
	c.t.Helper()
	c.record("Rename(%q, %q)", oldname, newname)
	err := c.fsys.Rename(oldname, newname)
	if err == nil {
		c.ReportFailf("Rename(%q, %q) succeeded, want error", oldname, newname)
	}
	return err
}

func (c *C[T, F, Fs]) Rename(oldname string, newname string) {
	c.t.Helper()
	c.record("Rename(%q, %q)", oldname, newname)
	if err := c.fsys.Rename(oldname, newname); err != nil {
		c.ReportFailf("Rename(%q, %q): %v", oldname, newname, err)
	}
}

func (c *C[T, F, Fs]) StatE(name string) (fs.FileInfo, error) {
	c.t.Helper()
	c.record("Stat(%q)", name)
	info, err := c.fsys.Stat(name)
	if err == nil {
		c.ReportFailf("Stat(%q) succeeded, want error", name)
	}
	return info, err
}

func (c *C[T, F, Fs]) Stat(name string) fs.FileInfo {
	c.t.Helper()
	c.record("Stat(%q)", name)
	info, err := c.fsys.Stat(name)
	if err != nil {
		c.ReportFailf("Stat(%q): %v", name, err)
	}
	return info
}

func (c *C[T, F, Fs]) SymlinkE(oldname string, newname string) error {
	c.t.Helper()
	c.record("Symlink(%q, %q)", oldname, newname)
	err := c.fsys.Symlink(oldname, newname)
	if err == nil {
		c.ReportFailf("Symlink(%q, %q) succeeded, want error", oldname, newname)
	}
	return err
}

func (c *C[T, F, Fs]) Symlink(oldname string, newname string) {
	c.t.Helper()
	c.record("Symlink(%q, %q)", oldname, newname)
	if err := c.fsys.Symlink(oldname, newname); err != nil {
		c.ReportFailf("Symlink(%q, %q): %v", oldname, newname, err)
	}
}
