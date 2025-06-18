package overlay

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"maps"
	"os"
	"slices"
	"syscall"

	"github.com/ngicks/go-common/serr"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// Layers is type for static, read-only merged layers under top read/write layer.
type Layers []Layer

func doInUpperLayer[V comparable](ll Layers, operation func(idx int, l Layer, name string) (V, error), name string) (v V, err error) {
	if len(ll) == 0 {
		return *new(V), fs.ErrNotExist
	}

	var result V

	for i, l := range slices.Backward(ll) {
		result, err = operation(i, l, name)
		if err == nil {
			break
		}
		if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, ErrWhitedOut) {
			err = ErrWhitedOut
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return
		}
	}

	if result == *new(V) {
		err = fs.ErrNotExist
		return
	}
	return result, nil
}

func (ll Layers) LayerOf(name string) (idx int, layer Layer, info fs.FileInfo, err error) {
	info, err = doInUpperLayer(
		ll,
		func(idx_ int, l Layer, name string) (fs.FileInfo, error) {
			idx = idx_
			layer = l
			return l.Lstat(name)
		},
		name,
	)
	if err != nil {
		return -1, Layer{}, nil, err
	}
	return
}

// Lstat takes lstat from first existing file in layers.
func (ll Layers) Lstat(name string) (fs.FileInfo, error) {
	return doInUpperLayer(
		ll,
		func(_ int, l Layer, name string) (fs.FileInfo, error) {
			return l.Lstat(name)
		},
		name,
	)
}

func (ll Layers) ReadLink(name string) (string, error) {
	return doInUpperLayer(
		ll,
		func(_ int, l Layer, name string) (string, error) {
			return l.ReadLink(name)
		},
		name,
	)
}

func (ll Layers) Open(name string) (*layersFile, error) {
	if len(ll) == 0 {
		return nil, fs.ErrNotExist
	}

	var (
		files []vroot.File
		isDir bool
	)
	for _, l := range slices.Backward(ll) {
		info, err := l.Lstat(name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, ErrWhitedOut) {
				break
			}
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			break
		}
		f, err := l.Open(name)
		if err != nil {
			return nil, err
		}
		if len(files) != 0 {
			if !info.IsDir() {
				_ = f.Close()
				break
			}
		}
		files = append(files, f)
		if !info.IsDir() {
			break
		}
		isDir = true
	}

	if len(files) == 0 {
		return nil, fs.ErrNotExist
	}

	return &layersFile{cachedIsDir: isDir, files: files}, nil
}

// layersFile is concatenated files in layers.
type layersFile struct {
	cachedIsDir bool
	// 1 or multiple files.
	// len(files) > 1 only when files are directories.
	files []vroot.File
	// If files are directories
	// cache ReadDir result if the method is called.
	// For layers are assumed static.
	direntCache []fs.FileInfo
}

func (lf *layersFile) isDir() bool {
	return lf.cachedIsDir
}

func (lf *layersFile) topFile() vroot.File {
	return lf.files[0]
}

func (lf *layersFile) clearDirEnt() {
	lf.direntCache = nil
}

func (lf *layersFile) close() error {
	errs := make([]serr.PrefixErr, len(lf.files))
	for i, file := range lf.files {
		errs[i] = serr.PrefixErr{
			P: fmt.Sprintf("file %d: ", i),
			E: file.Close(),
		}
	}
	return serr.GatherPrefixed(errs)
}

func (lf *layersFile) readDir(topFile vroot.File) ([]fs.FileInfo, error) {
	if lf.direntCache != nil {
		return lf.direntCache, nil
	}

	direntsMap := make(map[string]fs.FileInfo)
	for f := range concat(lf.files, []vroot.File{topFile}) {
		if f == nil {
			continue
		}
		// seek back in case it already read
		_, _ = f.Seek(0, io.SeekStart)
		dirents, err := f.Readdir(-1)
		if err != nil && err != io.EOF {
			return nil, err
		}
		for _, dirent := range dirents {
			direntsMap[dirent.Name()] = dirent
		}
	}

	lf.direntCache = slices.SortedFunc(
		maps.Values(direntsMap),
		func(i1, i2 fs.FileInfo) int {
			return cmp.Compare(i1.Name(), i2.Name())
		},
	)

	return lf.direntCache, nil
}

func concat[V any](sl ...[]V) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, l := range sl {
			for _, ele := range l {
				if !yield(ele) {
					return
				}
			}
		}
	}
}
