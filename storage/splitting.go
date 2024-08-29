package storage

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/spf13/afero"
)

type SafeWriter struct {
	fsys   afero.Fs
	option fsutil.SafeWriteOption
}

func NewSafeWriter(fsys afero.Fs, option fsutil.SafeWriteOption) *SafeWriter {
	return &SafeWriter{
		fsys:   fsys,
		option: option,
	}
}

func (s *SafeWriter) Write(
	path string,
	perm fs.FileMode,
	r io.Reader,
	postProcesses ...fsutil.SafeWritePostProcess,
) error {
	return s.option.SafeWrite(s.fsys, path, perm, r, postProcesses...)
}

func (s *SafeWriter) WriteFs(
	dir string,
	perm fs.FileMode,
	src fs.FS,
	postProcesses ...fsutil.SafeWritePostProcess,
) error {
	return s.option.SafeWriteFs(s.fsys, dir, perm, src, postProcesses...)
}

func (s *SafeWriter) CleanTmp() error {
	return s.option.CleanTmp(s.fsys)
}

func (s *SafeWriter) Fsys() afero.Fs {
	return s.fsys
}

// ReadSplitter returns splitted readers sequentially.
type ReaderSplitter interface {
	// Size returns limit size.
	// io.Reader returned from Next reads up to this size.
	// The last reader may reads less than Size.
	Size() int
	// Next returns size limited readers.
	// If ok is true, r is non nil and r reads up to given size.
	// Next and r.Read are not goroutine safe.
	// Calling Next before r is fully consumed causes an undefined behavior.
	Next() (r io.Reader, ok bool)
}

// fusedReader wraps io.Reader and once R returns a non nil error,
// fusedReader is melted and any subsequent Read calls returns that error.
type fusedReader struct {
	R   io.Reader
	Err error
}

func (r *fusedReader) Read(p []byte) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}

	n, err := r.R.Read(p)
	if err != nil {
		r.Err = err
	}
	return n, err
}

func (r *fusedReader) Melted() bool {
	return r.Err != nil
}

type splitter struct {
	r    *fusedReader
	size uint
	buf  []byte
}

const minReadSize = 8 * 1024

// SplitReader returns ReaderSplitter splitting at size.
// It will panic if size is 0.
func SplitReader(r io.Reader, size uint) ReaderSplitter {
	if size == 0 {
		panic("0 size in SplitReader")
	}
	return &splitter{
		r:    &fusedReader{R: r},
		size: size,
		buf:  make([]byte, minReadSize),
	}
}

func (s *splitter) Size() int {
	return int(s.size)
}

func (s *splitter) Next() (r io.Reader, ok bool) {
	if s.r.Melted() {
		return nil, false
	}

	buf := s.buf
	if s.size < uint(len(buf)) {
		buf = buf[:s.size]
	}
	readAhead, err := s.r.Read(buf)
	buf = buf[:readAhead]

	if readAhead == 0 && errors.Is(err, io.EOF) {
		// In case reader returns (n > 0 , nil) while it has reached EOF,
		// next Read would returns 0, io.EOF
		return nil, false
	}
	return io.LimitReader(io.MultiReader(bytes.NewReader(buf), s.r), int64(s.size)), true
}

// PathModifierAppendIndex appends path with "_" + i.
// i will be padded with "0" to be 3 digits.
// If i > 999 or i < -99, number will be 4 digits or 3 digits with minus sign.
//
// PathModifierAppendIndex removes filepath.Separator from tail
// if path is suffixed with it.
func PathModifierAppendIndex(path string, i int) string {
	path, _ = strings.CutSuffix(path, string(filepath.Separator))
	return fmt.Sprintf("%s_%03d", path, i)
}

func WriteSplitting(
	fsys afero.Fs,
	opt fsutil.SafeWriteOption,
	path string,
	perm fs.FileMode,
	r io.Reader,
	size uint,
	pathModifier func(path string, i int) string,
	trapper func(path string, r io.Reader) io.Reader,
) ([]string, error) {
	splitter := SplitReader(r, size)

	if pathModifier == nil {
		pathModifier = PathModifierAppendIndex
	}

	var out []string
	seen := map[string]bool{}
	var i int
	for {
		r, ok := splitter.Next()
		if !ok {
			break
		}

		nextPath := filepath.Clean(pathModifier(path, i))
		if seen[nextPath] {
			return out, fmt.Errorf("duplicate name: %s", nextPath)
		}
		seen[nextPath] = true

		if trapper != nil {
			r = trapper(nextPath, r)
		}

		i++

		err := opt.SafeWrite(fsys, nextPath, perm, r)
		if err != nil {
			return out, err
		}

		out = append(out, nextPath)
	}

	return out, nil
}

type SplittingStorage struct {
	fileFsys     *SafeWriter
	metadataFsys *SafeWriter
	hashAlgo     crypto.Hash
	splitSize    uint
	pathModifier func(s string, i int) string
}

func NewSplittingStorage(
	fileFsys *SafeWriter,
	metadataFsys *SafeWriter,
	splitSize uint,
	pathModifier func(s string, i int) string,
	safeWriteOption fsutil.SafeWriteOption,
) *SplittingStorage {
	return &SplittingStorage{
		fileFsys:     fileFsys,
		metadataFsys: metadataFsys,
		splitSize:    splitSize,
		pathModifier: pathModifier,
	}
}

type SplittedFileMetadata struct {
	Total    SplittedFileHash
	Splitted []SplittedFileHash
}

type SplittedFileHash struct {
	Path     string
	Size     int
	HashSum  string
	HashAlgo string
}

const (
	metaSuffix = ".meta.json"
)

type readSizeCounter struct {
	R io.Reader
	N atomic.Int64
}

func (r *readSizeCounter) Read(p []byte) (int, error) {
	n, err := r.R.Read(p)
	r.N.Add(int64(n))
	return n, err
}

type splittedDataSet struct {
	H    hash.Hash
	C    *readSizeCounter
	Path string
}

func (s *SplittingStorage) Write(path string, perm fs.FileMode, r io.Reader) ([]string, error) {
	path = filepath.Clean(path)

	f, err := s.metadataFsys.fsys.Open(path + metaSuffix)
	if err == nil {
		var meta SplittedFileMetadata
		err := json.NewDecoder(f).Decode(&meta)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, s := range meta.Splitted {
			paths = append(paths, s.Path)
		}
		return paths, nil
	} else {
		_ = f.Close()
	}

	hTotal := s.hashAlgo.New()
	cTotal := &readSizeCounter{R: io.TeeReader(r, hTotal)}

	sets := make([]splittedDataSet, 0)
	paths, err := WriteSplitting(
		s.fileFsys.fsys,
		s.fileFsys.option,
		path,
		perm,
		cTotal,
		s.splitSize,
		s.pathModifier,
		func(path string, r io.Reader) io.Reader {
			h := s.hashAlgo.New()
			r = io.TeeReader(r, h)
			sizeCounted := &readSizeCounter{R: r}
			sets = append(sets, splittedDataSet{
				H:    h,
				C:    sizeCounted,
				Path: filepath.Clean(path),
			})
			return sizeCounted
		},
	)
	if err != nil {
		return paths, err
	}

	meta := SplittedFileMetadata{
		Total: SplittedFileHash{
			Path:     path,
			Size:     int(cTotal.N.Load()),
			HashSum:  hex.EncodeToString(hTotal.Sum(nil)),
			HashAlgo: s.hashAlgo.String(),
		},
		Splitted: mapToSplittedFileHash(sets, s.hashAlgo),
	}

	bin, _ := json.Marshal(meta)
	err = s.metadataFsys.Write(
		path+metaSuffix,
		fs.ModePerm,
		bytes.NewReader(bin),
	)
	if err != nil {
		return paths, err
	}

	return paths, nil
}

func mapToSplittedFileHash(sets []splittedDataSet, algo crypto.Hash) []SplittedFileHash {
	out := make([]SplittedFileHash, len(sets))
	for i, set := range sets {
		out[i] = SplittedFileHash{
			Path:     set.Path,
			Size:     int(set.C.N.Load()),
			HashSum:  hex.EncodeToString(set.H.Sum(nil)),
			HashAlgo: algo.String(),
		}
	}
	return out
}

type closable struct {
	io.Reader
	closer []io.Closer
}

func (c *closable) Close() error {
	var lastErr error
	for _, c := range c.closer {
		err := c.Close()
		if err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *SplittingStorage) Read(path string) (r io.ReadCloser, size int, err error) {
	f, err := s.metadataFsys.fsys.Open(filepath.Clean(path) + metaSuffix)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var meta SplittedFileMetadata
	dec := json.NewDecoder(f)
	err = dec.Decode(&meta)
	_ = f.Close()
	if err != nil {

		return nil, 0, err
	}

	closable := &closable{}
	var readers []io.Reader
	for _, p := range meta.Splitted {
		f, err := s.fileFsys.fsys.Open(p.Path)
		if err != nil {
			_ = closable.Close()
			return nil, 0, err
		}
		readers = append(readers, f)
		closable.closer = append(closable.closer, f)
	}

	closable.Reader = io.MultiReader(readers...)

	return closable, meta.Total.Size, nil
}
