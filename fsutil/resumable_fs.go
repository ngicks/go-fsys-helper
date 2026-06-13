package fsutil

// Fs-backed adapters that implement [ResumableSource] and [ResumableSink],
// letting one filesystem act as the remote side of Pull/Push.  Together they
// make the whole resumable machinery testable fsys↔fsys without any network
// involvement.

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
)

// FsSource adapts a regular file inside a filesystem into a [ResumableSource].
// It is useful for testing [ResumableCopyOption.Pull] without a real network
// source, and as a building block for fsys-backed transfer pipelines.
//
// Concurrent calls to Open are safe as long as the underlying filesystem
// supports concurrent OpenFile calls.
type FsSource[
	Fsys interface {
		OpenFileFs[File]
		StatFs
	},
	File interface {
		ReadFile
		SeekFile
		CloseFile
	},
] struct {
	fsys Fsys
	name string
	etag string
}

// NewFsSource returns a [FsSource] that serves the file at name inside fsys.
// etag is the content-identity token to advertise in the returned
// [ContentInfo]; pass "" when the identity is unknown.
func NewFsSource[
	Fsys interface {
		OpenFileFs[File]
		StatFs
	},
	File interface {
		ReadFile
		SeekFile
		CloseFile
	},
](fsys Fsys, name string, etag string) *FsSource[Fsys, File] {
	return &FsSource[Fsys, File]{fsys: fsys, name: name, etag: etag}
}

// Open implements [ResumableSource].  It stats the file for its current size,
// opens it, seeks to offset, and returns a reader whose first byte is at
// offset.  The returned [ContentInfo] carries the ETag given to [NewFsSource]
// and the file's current size.
func (s *FsSource[Fsys, File]) Open(
	ctx context.Context,
	offset int64,
) (io.ReadCloser, ContentInfo, error) {
	_ = ctx
	fi, err := s.fsys.Stat(s.name)
	if err != nil {
		return nil, ContentInfo{}, WrapPathErr("stat", s.name, err)
	}
	size := fi.Size()

	f, err := s.fsys.OpenFile(s.name, os.O_RDONLY, 0)
	if err != nil {
		return nil, ContentInfo{}, WrapPathErr("open", s.name, err)
	}

	if offset > 0 {
		if _, serr := f.Seek(offset, io.SeekStart); serr != nil {
			_ = f.Close()
			return nil, ContentInfo{}, WrapPathErr("seek", s.name, serr)
		}
	}

	return f, ContentInfo{ETag: s.etag, Size: size}, nil
}

// fsSinkFile is the minimal file interface required by [FsSink]. On top of
// what [ResumableCopyOption] needs, the file must implement [TruncateFile] so
// that FsSink can shorten an oversized part file during resume without
// re-creating it.
type fsSinkFile interface {
	resumableFile
	TruncateFile
}

// fsSinkFsys is the minimal filesystem interface required by [FsSink].
type fsSinkFsys[File fsSinkFile] interface {
	OpenFileFs[File]
	StatFs
	RenameFs
	RemoveFs
}

// FsSink adapts a destination file in a filesystem into a [ResumableSink]
// using the same .part + sidecar + rename semantics as
// [ResumableCopyOption.Pull]'s local side.
//
// The PartSuffix field overrides the default ".part" in-progress suffix.
//
// State, Append, and Commit calls are NOT safe for concurrent use on the same
// FsSink instance; the caller is responsible for serialisation.
type FsSink[Fsys fsSinkFsys[File], File fsSinkFile] struct {
	fsys Fsys
	name string
	perm fs.FileMode
	// PartSuffix overrides the default ".part" suffix for the in-progress file.
	// The ETag sidecar is stored at partPath + ".etag".
	PartSuffix string
}

// NewFsSink returns a [FsSink] that writes the file at name inside fsys.
// perm is the file-creation mode used when creating the part file (and
// therefore the committed final file after rename).
func NewFsSink[Fsys fsSinkFsys[File], File fsSinkFile](
	fsys Fsys,
	name string,
	perm fs.FileMode,
) *FsSink[Fsys, File] {
	return &FsSink[Fsys, File]{fsys: fsys, name: name, perm: perm}
}

func (s *FsSink[Fsys, File]) sfx() string {
	if s.PartSuffix != "" {
		return s.PartSuffix
	}
	return defaultPartSuffix
}

// partPath and sidecarPath delegate to the shared partPaths naming used by
// Pull, keeping the two sides of a resumable transfer symmetric.
func (s *FsSink[Fsys, File]) partPath() string {
	pp, _ := partPaths(s.name, s.sfx())
	return pp
}

func (s *FsSink[Fsys, File]) sidecarPath() string {
	_, sc := partPaths(s.name, s.sfx())
	return sc
}

// readSidecar reads the ETag sidecar, returning ("", nil) on not-exist.
func (s *FsSink[Fsys, File]) readSidecar() (string, error) {
	return readSidecar[Fsys, File](s.fsys, s.sidecarPath())
}

// writeSidecar writes etag to the sidecar through the fsys, truncating any
// previous content.
func (s *FsSink[Fsys, File]) writeSidecar(etag string) error {
	return writeSidecar[Fsys, File](s.fsys, s.sidecarPath(), etag)
}

// State implements [ResumableSink.State].
//
// It returns:
//   - {Offset: finalSize, Complete: true} when the final file exists.
//   - {Offset: partSize, ETag: storedETag} when only the part file exists.
//   - Zero [SinkState] when neither exists.
func (s *FsSink[Fsys, File]) State(ctx context.Context) (SinkState, error) {
	_ = ctx
	// Check for the committed final file first.
	if fi, err := s.fsys.Stat(s.name); err == nil {
		return SinkState{Offset: fi.Size(), Complete: true}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return SinkState{}, WrapPathErr("stat", s.name, err)
	}

	// Check for the part file.
	fi, err := s.fsys.Stat(s.partPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SinkState{}, nil
		}
		return SinkState{}, WrapPathErr("stat", s.partPath(), err)
	}

	etag, err := s.readSidecar()
	if err != nil {
		// An unreadable (as opposed to absent) sidecar must not silently
		// disable the pusher's ETag-mismatch guard.
		return SinkState{}, WrapPathErr("read-sidecar", s.sidecarPath(), err)
	}
	return SinkState{Offset: fi.Size(), ETag: etag}, nil
}

// Append implements [ResumableSink.Append].
//
// When offset == 0, the part file is created or truncated to zero.  When
// offset > 0, the existing part file must have at least offset bytes; if it is
// longer it is truncated to offset; if it is shorter, [ErrSizeMismatch] is
// returned.  info.ETag is persisted to the sidecar before any data is written.
func (s *FsSink[Fsys, File]) Append(
	ctx context.Context,
	info ContentInfo,
	offset int64,
	r io.Reader,
) error {
	pp := s.partPath()

	var (
		f   File
		err error
	)

	if offset == 0 {
		f, err = s.fsys.OpenFile(pp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, s.perm)
		if err != nil {
			return WrapPathErr("open-part", pp, err)
		}
	} else {
		f, err = s.fsys.OpenFile(pp, os.O_WRONLY|os.O_CREATE, s.perm)
		if err != nil {
			return WrapPathErr("open-part", pp, err)
		}
		fi, sterr := s.fsys.Stat(pp)
		if sterr != nil {
			_ = f.Close()
			return WrapPathErr("stat-part", pp, sterr)
		}
		partSize := fi.Size()
		switch {
		case partSize > offset:
			if terr := f.Truncate(offset); terr != nil {
				_ = f.Close()
				return WrapPathErr("truncate-part", pp, terr)
			}
		case partSize < offset:
			_ = f.Close()
			return WrapPathErr("append", pp, ErrSizeMismatch)
		}
		if _, serr := f.Seek(offset, io.SeekStart); serr != nil {
			_ = f.Close()
			return WrapPathErr("seek-part", pp, serr)
		}
	}

	// Persist ETag to sidecar before writing any data.
	if info.ETag != "" {
		if werr := s.writeSidecar(info.ETag); werr != nil {
			_ = f.Close()
			return WrapPathErr("write-sidecar", s.sidecarPath(), werr)
		}
	}

	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return WrapPathErr("append-copy", pp, copyErr)
	}
	// On network-backed filesystems Close flushes the final write; its error
	// is the only signal of a lost write, so give it full path context too.
	return WrapPathErr("close-part", pp, closeErr)
}

// Commit implements [ResumableSink.Commit].
//
// It renames the part file to the final path and removes the sidecar.  If the
// final file already exists, Commit keeps it and discards any leftover part
// file and sidecar instead of replacing it (idempotent: a retried Commit after
// a partially-cleaned-up earlier success never clobbers committed content).
func (s *FsSink[Fsys, File]) Commit(ctx context.Context) error {
	_ = ctx
	pp := s.partPath()
	sc := s.sidecarPath()

	_, finalErr := s.fsys.Stat(s.name)
	switch {
	case finalErr == nil:
		// Already committed; clean up any leftovers from an interrupted
		// earlier attempt.
		if rerr := s.fsys.Remove(pp); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return WrapPathErr("remove-part", pp, rerr)
		}
		_ = s.fsys.Remove(sc) // ignore not-exist
		return nil
	case !errors.Is(finalErr, fs.ErrNotExist):
		return WrapPathErr("stat", s.name, finalErr)
	}

	if err := s.fsys.Rename(pp, s.name); err != nil {
		return WrapLinkErr("rename", pp, s.name, err)
	}
	_ = s.fsys.Remove(sc) // ignore not-exist
	return nil
}
