package fsutil

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ngicks/go-fsys-helper/stream"
	"github.com/spf13/afero"
)

type nonRegularFileHandling string

const (
	nonRegularFileHandlingError  nonRegularFileHandling = "" // default is to return an error.
	nonRegularFileHandlingIgnore nonRegularFileHandling = "ignore"
	// nonRegularFileHandlingTrySymlink nonRegularFileHandling = "try_symlink"
)

type copyFsOption struct {
	handleNonRegularFile nonRegularFileHandling
	chmodIf              func(path string) (perm fs.FileMode, ok bool)
	noChmod              bool
	ctx                  context.Context
}

func newCopyFsOption(opts ...CopyFsOption) copyFsOption {
	opt := copyFsOption{}
	for _, o := range opts {
		o(&opt)
	}
	return opt
}

func (o copyFsOption) isCancelled() error {
	if o.ctx != nil && o.ctx.Err() != nil {
		return o.ctx.Err()
	}
	return nil
}

type CopyFsOption func(o *copyFsOption)

func CopyFsWithIgnoreNonRegularFile() CopyFsOption {
	return func(o *copyFsOption) {
		o.handleNonRegularFile = nonRegularFileHandlingIgnore
	}
}

func CopyFsWithNoChmod(noChmod bool) CopyFsOption {
	return func(o *copyFsOption) {
		o.noChmod = noChmod
	}
}

func CopyFsWithOverridePermission(chmodIf func(path string) (perm fs.FileMode, ok bool)) CopyFsOption {
	return func(o *copyFsOption) {
		o.chmodIf = chmodIf
	}
}

func CopyFsWithContext(ctx context.Context) CopyFsOption {
	return func(o *copyFsOption) {
		o.ctx = ctx
	}
}

// CopyFS copies from fs.FS to afero.FS.
//
// The default behavior of CopyFS is:
//   - returns an error if src contains non regular files
//   - copies permission bits
//   - makes directories with fs.ModePerm (0o777) before umask.
func CopyFS(dst afero.Fs, src fs.FS, opts ...CopyFsOption) error {
	// in case that the file type of dst does not implement io.ReaderFrom or
	// the file type of src does not implement io.WriterTo.
	// Use 64KiB buffers and reuse them across this package to gain some performance boost.
	buf := getBuf()
	defer putBuf(buf)

	opt := newCopyFsOption(opts...)

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		if err := opt.isCancelled(); err != nil {
			return err
		}

		return copyPath(dst, src, path, opt, buf)
	})

	if err != nil {
		return fmt.Errorf("fsutil.CopyFS: %w", err)
	}
	return nil
}

// CopyFsPath copies contents of src specified by paths into dst.
func CopyFsPath(dst afero.Fs, src fs.FS, paths []string, opts ...CopyFsOption) error {
	buf := getBuf()
	defer putBuf(buf)

	opt := newCopyFsOption(opts...)

	for _, p := range paths {
		if err := opt.isCancelled(); err != nil {
			return err
		}

		p = path.Clean(filepath.ToSlash(p))

		if strings.HasPrefix(p, "..") {
			return fmt.Errorf("fsutil.CopyFsPath: path is out of src, path = %s: %w", p, fs.ErrNotExist)
		}

		var err error

		// Should we avoid creating "."?
		err = dst.MkdirAll(filepath.FromSlash(path.Dir(p)), fs.ModePerm)
		if err != nil {
			return fmt.Errorf("fsutil.CopyFsPath: mkdirAll: %w", err)
		}

		err = copyPath(dst, src, p, opt, buf)
		if err != nil {
			return fmt.Errorf("fsutil.CopyFsPath: %w", err)
		}
	}

	return nil
}

func copyPath(dst afero.Fs, src fs.FS, p string, opt copyFsOption, buf *[]byte) error {
	target := filepath.FromSlash(p)

	r, err := src.Open(p)
	if err != nil {
		return err
	}
	closeROnce := once(func() error { return r.Close() })
	defer func() { _ = closeROnce() }()

	rInfo, err := r.Stat()
	if err != nil {
		return err
	}

	chmod := func() error {
		perm := rInfo.Mode().Perm()

		var ok bool
		if opt.chmodIf != nil {
			var overridden fs.FileMode
			overridden, ok = opt.chmodIf(p)
			if ok {
				perm = overridden
			}
		}

		if ok || !opt.noChmod {
			err = dst.Chmod(target, perm)
			if err != nil {
				return fmt.Errorf("failed to chmod created dir, target = %s, err = %w", target, err)
			}
		}
		return nil
	}

	if rInfo.IsDir() {
		if err := dst.MkdirAll(target, fs.ModePerm); err != nil {
			return err
		}
		if err := chmod(); err != nil {
			return err
		}
		return nil
	}

	if !rInfo.Mode().IsRegular() {
		switch opt.handleNonRegularFile {
		case nonRegularFileHandlingError: // default
			return fmt.Errorf("%w: non regular file is not supported.", ErrBadInput)
		case nonRegularFileHandlingIgnore:
			return nil
			// case nonRegularFileHandlingTrySymlink:
		}
	}

	w, err := dst.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.ModePerm)
	if err != nil {
		return err
	}
	closeWOnce := once(w.Close)
	defer func() { _ = closeWOnce() }()

	err = chmod()
	if err != nil {
		return err
	}

	var rr io.Reader = r
	if opt.ctx != nil {
		rr = stream.NewCancellable(opt.ctx, r)
	}
	if n, err := io.CopyBuffer(w, rr, *buf); err != nil {
		return fmt.Errorf("copying %s, %w at %d", p, err, n)
	}

	if err := closeROnce(); err != nil {
		return err
	}

	if err := w.Sync(); err != nil {
		return err
	}

	if err := closeWOnce(); err != nil {
		return err
	}

	return nil
}

// once wraps fn so that it would only be called once.
// It is a goroutine unsafe version of sync.OnceValue.
// It also omits panic-propagation feature,
// since panic is assumed not to be recovered by callers themselves.
func once[T any](fn func() T) func() T {
	called := false
	var result T
	return func() T {
		if called {
			return result
		}
		called = true
		result = fn()
		return result
	}
}
