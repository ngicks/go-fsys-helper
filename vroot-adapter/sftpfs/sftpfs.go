// Package sftpfs implements [vroot.Unrooted] over a *sftp.Client.
package sftpfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/pkg/sftp"
)

var _ vroot.Fs[vroot.File] = (*SftpFs)(nil)

type SftpFs struct {
	client      *sftp.Client
	posixRename bool
	base        string
}

// New returns an [*SftpFs] rooted at base. base must be an absolute
// peer-side POSIX path; it is not validated against the remote.
func New(c *sftp.Client, posixRename bool, base string) *SftpFs {
	return &SftpFs{client: c, posixRename: posixRename, base: base}
}

// Client returns the underlying *sftp.Client (for advanced use).
func (s *SftpFs) Client() *sftp.Client { return s.client }

// Base returns the absolute peer-side base directory.
func (s *SftpFs) Base() string { return s.base }

// resolvePath cleans name, rejects path-traversal escapes, and joins
// with Base. Mirrors [osfs.Unrooted.resolvePath] but operates on POSIX
// paths.
func (s *SftpFs) resolvePath(name string) (string, error) {
	if s == nil || s.base == "" {
		panic("calling method of zero *SftpFs")
	}

	cleaned := path.Clean(name)
	if cleaned == "." {
		return s.base, nil
	}
	if !isLocalSlash(cleaned) {
		return "", vroot.ErrPathEscapes
	}
	return path.Join(s.base, cleaned), nil
}

// isLocalSlash reports whether p (already path.Clean'd) is a
// POSIX-style local path: not absolute, and does not escape its parent
// via "..".
func isLocalSlash(p string) bool {
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return false
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return false
	}
	return true
}

// Chmod implements [vroot.Fs].
func (s *SftpFs) Chmod(name string, mode fs.FileMode) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chmod", name, err)
	}
	return s.client.Chmod(abs, mode)
}

// Chown implements [vroot.Fs].
func (s *SftpFs) Chown(name string, uid, gid int) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chown", name, err)
	}
	return s.client.Chown(abs, uid, gid)
}

// Chtimes implements [vroot.Fs].
func (s *SftpFs) Chtimes(name string, atime, mtime time.Time) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chtimes", name, err)
	}
	return s.client.Chtimes(abs, atime, mtime)
}

// Close implements [vroot.Fs]. It does not close the underlying
// *sftp.Client — that is the caller's job. Returning nil keeps
// fsutil's safe-write code path happy.
func (s *SftpFs) Close() error { return nil }

// Create implements [vroot.Fs].
func (s *SftpFs) Create(name string) (vroot.File, error) {
	return s.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// Lchown implements [vroot.Fs] best-effort via Chown; sftp does not
// distinguish symlink-aware chown.
func (s *SftpFs) Lchown(name string, uid, gid int) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("lchown", name, err)
	}
	return s.client.Chown(abs, uid, gid)
}

// Link implements [vroot.Fs] (hardlink, where supported).
func (s *SftpFs) Link(oldname, newname string) error {
	oldAbs, err := s.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	newAbs, err := s.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	return s.client.Link(oldAbs, newAbs)
}

// Lstat implements [vroot.Fs].
func (s *SftpFs) Lstat(name string) (fs.FileInfo, error) {
	abs, err := s.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	if abs == s.base {
		return s.client.Stat(abs)
	}
	return s.client.Lstat(abs)
}

// Mkdir implements [vroot.Fs]. perm is best-effort applied via
// Chmod after the mkdir (sftp does not transmit perm directly).
func (s *SftpFs) Mkdir(name string, perm fs.FileMode) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	if err := s.client.Mkdir(abs); err != nil {
		return mapSftpErr(err)
	}
	if perm != 0 {
		_ = s.client.Chmod(abs, perm)
	}
	return nil
}

// MkdirAll implements [vroot.Fs].
func (s *SftpFs) MkdirAll(name string, perm fs.FileMode) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	if err := s.client.MkdirAll(abs); err != nil {
		return err
	}
	if perm != 0 {
		_ = s.client.Chmod(abs, perm)
	}
	return nil
}

// Name implements [vroot.Fs].
func (s *SftpFs) Name() string { return "sftp:" + s.base }

// Open implements [vroot.Fs].
func (s *SftpFs) Open(name string) (vroot.File, error) {
	return s.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile implements [vroot.Fs]. perm is best-effort applied via
// Chmod after the open succeeds.
//
// O_APPEND is honored by seeking to end of file after open because pkg/sftp's
// stock Server does not implement append-on-write — it ignores SSH_FXF_APPEND
// and expects the client to track offsets. See pkg/sftp/server.go:494.
func (s *SftpFs) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	abs, err := s.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	f, err := s.client.OpenFile(abs, flag)
	if err != nil {
		return nil, mapSftpErr(err)
	}
	if flag&os.O_APPEND != 0 {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			_ = f.Close()
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	}
	if perm != 0 && (flag&os.O_CREATE != 0) {
		_ = s.client.Chmod(abs, perm)
	}
	return &sftpFile{File: f, client: s.client}, nil
}

// ReadLink implements [vroot.Fs].
func (s *SftpFs) ReadLink(name string) (string, error) {
	abs, err := s.resolvePath(name)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	if abs == s.base {
		return "", fsutil.WrapPathErr("readlink", abs, syscall.EINVAL)
	}
	return s.client.ReadLink(abs)
}

// Remove implements [vroot.Fs].
func (s *SftpFs) Remove(name string) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("remove", name, err)
	}
	return s.client.Remove(abs)
}

// RemoveAll implements [vroot.Fs] via a recursive walk. The common
// case (Remove on a regular file) hits the fast path.
func (s *SftpFs) RemoveAll(name string) error {
	abs, err := s.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("removeall", name, err)
	}
	if abs == s.base {
		return fsutil.WrapPathErr("removeall", ".", fs.ErrInvalid)
	}
	if err := s.client.Remove(abs); err == nil {
		return nil
	} else if isSftpNotExist(err) {
		return nil
	}
	fi, err := s.client.Lstat(abs)
	if err != nil {
		if isSftpNotExist(err) {
			return nil
		}
		return err
	}
	if !fi.IsDir() {
		return s.client.Remove(abs)
	}
	w := s.client.Walk(abs)
	var paths []string
	for w.Step() {
		paths = append(paths, w.Path())
	}
	for i := len(paths) - 1; i >= 0; i-- {
		fi, err := s.client.Lstat(paths[i])
		if err != nil {
			continue
		}
		if fi.IsDir() {
			_ = s.client.RemoveDirectory(paths[i])
		} else {
			_ = s.client.Remove(paths[i])
		}
	}
	return nil
}

// Rename implements [vroot.Fs] using POSIX rename when supported.
func (s *SftpFs) Rename(oldname, newname string) error {
	oldAbs, err := s.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	newAbs, err := s.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}

	if s.posixRename {
		return s.client.PosixRename(oldAbs, newAbs)
	}
	return s.client.Rename(oldAbs, newAbs)
}

// Stat implements [vroot.Fs].
func (s *SftpFs) Stat(name string) (fs.FileInfo, error) {
	abs, err := s.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return s.client.Stat(abs)
}

// Symlink implements [vroot.Fs]. oldname (the link target) is stored
// verbatim — symlink targets may legitimately be relative or point
// outside the root, and containment is the remote's job.
func (s *SftpFs) Symlink(oldname, newname string) error {
	newAbs, err := s.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	return mapSftpErr(s.client.Symlink(oldname, newAbs))
}

// ReadDir implements the [vroot.ReadDirFs] optional optimization,
// adapting sftp's []os.FileInfo to []fs.DirEntry.
func (s *SftpFs) ReadDir(name string) ([]fs.DirEntry, error) {
	abs, err := s.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("readdir", name, err)
	}
	fis, err := s.client.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, len(fis))
	for i, fi := range fis {
		out[i] = sftpDirEntry{fi: fi}
	}
	return out, nil
}

// sftpFile wraps *sftp.File to satisfy [vroot.File]. Directory iteration is
// implemented out-of-band via Client.ReadDir(path), since pkg/sftp's File
// does not expose Readdir.
type sftpFile struct {
	*sftp.File
	client *sftp.Client

	dirMu      sync.Mutex
	dirEntries []os.FileInfo
	dirLoaded  bool
	dirErr     error
}

func (f *sftpFile) Fd() uintptr { return ^uintptr(0) }

func (f *sftpFile) WriteString(s string) (int, error) {
	return f.Write([]byte(s))
}

// Sync forwards to *sftp.File.Sync. Servers that lack the fsync@openssh.com
// extension (notably pkg/sftp's stock NewServer; OpenSSH's sftp-server has it)
// surface that as SSH_FX_OP_UNSUPPORTED — treat that as a successful no-op
// rather than an error, matching how in-memory file systems behave.
func (f *sftpFile) Sync() error {
	err := f.File.Sync()
	if err == nil {
		return nil
	}
	stErr, ok := errors.AsType[*sftp.StatusError](err)
	if ok && stErr.FxCode() == sftp.ErrSSHFxOpUnsupported {
		return nil
	}
	return err
}

// loadDir fetches the directory listing once and caches it for paginated
// readers below.
func (f *sftpFile) loadDir() error {
	f.dirMu.Lock()
	defer f.dirMu.Unlock()
	if f.dirLoaded {
		return f.dirErr
	}
	fis, err := f.client.ReadDir(f.Name())
	f.dirLoaded = true
	f.dirEntries = fis
	f.dirErr = err
	return err
}

// Readdir mirrors [os.File.Readdir] semantics: n>0 returns up to n entries
// (io.EOF when exhausted); n<=0 returns all remaining entries at once.
func (f *sftpFile) Readdir(n int) ([]fs.FileInfo, error) {
	if err := f.loadDir(); err != nil {
		return nil, err
	}
	f.dirMu.Lock()
	defer f.dirMu.Unlock()
	if n <= 0 {
		out := f.dirEntries
		f.dirEntries = nil
		return out, nil
	}
	if len(f.dirEntries) == 0 {
		return nil, io.EOF
	}
	if n > len(f.dirEntries) {
		n = len(f.dirEntries)
	}
	out := f.dirEntries[:n]
	f.dirEntries = f.dirEntries[n:]
	return out, nil
}

func (f *sftpFile) Readdirnames(n int) ([]string, error) {
	fis, err := f.Readdir(n)
	names := make([]string, len(fis))
	for i, fi := range fis {
		names[i] = fi.Name()
	}
	return names, err
}

func (f *sftpFile) ReadDir(n int) ([]fs.DirEntry, error) {
	fis, err := f.Readdir(n)
	out := make([]fs.DirEntry, len(fis))
	for i, fi := range fis {
		out[i] = sftpDirEntry{fi: fi}
	}
	return out, err
}

// sftpDirEntry adapts os.FileInfo to fs.DirEntry.
type sftpDirEntry struct{ fi os.FileInfo }

func (e sftpDirEntry) Name() string               { return e.fi.Name() }
func (e sftpDirEntry) IsDir() bool                { return e.fi.IsDir() }
func (e sftpDirEntry) Type() fs.FileMode          { return e.fi.Mode().Type() }
func (e sftpDirEntry) Info() (fs.FileInfo, error) { return e.fi, nil }

// isSftpNotExist returns true if err represents a missing-file
// status, regardless of whether it was wrapped via fs.PathError or
// surfaced as a sftp.StatusError.
func isSftpNotExist(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	stErr, ok := errors.AsType[*sftp.StatusError](err)
	if ok && stErr.FxCode() == sftp.ErrSSHFxNoSuchFile {
		return true
	}
	return false
}

// mapSftpErr normalizes server-side errors that the pkg/sftp protocol cannot
// represent precisely (SFTP v3 has no SSH_FX_FILE_ALREADY_EXISTS), so callers
// can use [errors.Is] against [fs.ErrExist].
//
// The detection looks at the server-supplied message because pkg/sftp's
// statusFromError loses errno fidelity for EEXIST: it collapses to
// SSH_FX_FAILURE with the os.PathError text preserved verbatim.
func mapSftpErr(err error) error {
	if err == nil {
		return nil
	}
	stErr, ok := errors.AsType[*sftp.StatusError](err)
	if !ok || stErr.FxCode() != sftp.ErrSSHFxFailure {
		return err
	}
	msg := stErr.Error()
	if strings.Contains(msg, "file exists") || strings.Contains(msg, "file already exists") {
		return errors.Join(err, fs.ErrExist)
	}
	return err
}
