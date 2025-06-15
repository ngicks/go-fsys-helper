package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrBadPattern = errors.New("bad pattern")
	ErrMaxRetry   = errors.New("max retry")
)

func OpenFileRandom[FS OpenFileFs[File], File any](fsys FS, dir string, pattern string, perm fs.FileMode) (File, error) {
	return openRandom(
		fsys,
		dir,
		pattern,
		perm,
		func(fsys FS, name string, perm fs.FileMode) (File, error) {
			return fsys.OpenFile(filepath.FromSlash(name), os.O_RDWR|os.O_CREATE|os.O_EXCL, perm|0o200) // at least writable
		},
	)
}

func MkdirRandom[FS interface {
	OpenFileFs[File]
	MkdirFs
}, File any](fsys FS, dir string, pattern string, perm fs.FileMode) (File, error) {
	return openRandom(
		fsys,
		dir,
		pattern,
		perm,
		func(fsys FS, name string, perm fs.FileMode) (File, error) {
			err := fsys.Mkdir(name, perm)
			if err != nil {
				return *new(File), err
			}
			return fsys.OpenFile(name, os.O_RDONLY, 0)
		},
	)
}

func openRandom[FS, File any](
	fsys FS,
	dir string,
	pattern string,
	perm fs.FileMode,
	open func(fsys FS, name string, perm fs.FileMode) (File, error),
) (File, error) {
	if dir == "" {
		dir = "." + string(filepath.Separator)
	}

	if strings.Contains(pattern, string(filepath.Separator)) {
		return *new(File), fmt.Errorf("%w: %q contains path separators", ErrBadPattern, pattern)
	}

	var prefix, suffix string
	if i := strings.LastIndex(pattern, "*"); i < 0 {
		prefix = pattern
	} else {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}

	attempt := 0
	for {
		random := randomUint32Padded()
		name := filepath.Join(dir, prefix+random+suffix)
		f, err := open(fsys, name, perm.Perm())
		if err == nil {
			return f, nil
		}
		if errors.Is(err, fs.ErrExist) {
			attempt++
			if attempt < 10000 {
				continue
			} else {
				return *new(File), fmt.Errorf(
					"%w: opening %s",
					ErrMaxRetry, path.Join(dir, prefix+"*"+suffix),
				)
			}
		} else {
			return *new(File), err
		}
	}
}

// randomUint32Padded return math/rand/v2.Uint32 as left-0-padded string.
// The returned string always satisfies len(s) == 10 and '0' <= s[i] <= '9'.
func randomUint32Padded() string {
	// os.MkdiTemp does this thing. Just shadowing the behavior.
	// But there's no strong opinion about this;
	// It can be longer, or even shorter. We can expand this to
	// 9999999999 instead of 4294967295.
	s := strconv.FormatUint(uint64(rand.Uint32()), 10)
	var builder strings.Builder
	builder.Grow(len("4294967295"))
	r := len("4294967295") - len(s)
	for range r {
		builder.WriteByte('0')
	}
	builder.WriteString(s)
	return builder.String()
}
