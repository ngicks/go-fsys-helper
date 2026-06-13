package fsutil

// Package-level resumable pull/push implementation.
//
// Pull downloads content from a [ResumableSource] into a local file, resuming
// from a previously interrupted transfer when possible.  Push uploads a local
// file to a [ResumableSink], also supporting resume.  Both operations use a
// ".part" work-in-progress file and an ETag sidecar ("<name>.part.etag") to
// track identity across restarts, and commit via an atomic rename.
//
// Parent directories of the destination must exist before calling Pull or Push;
// neither function creates them.

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
)

// ContentInfo describes a piece of content being transferred.
// The zero value represents content with unknown identity and unknown size.
type ContentInfo struct {
	// ETag is an opaque content-identity token.  The empty string means
	// "unknown or unsupported".  Two equal, non-empty ETags guarantee the
	// bytes are identical — for content-addressable stores this is typically
	// a digest string such as "sha256:<hex>".
	ETag string
	// Size is the total content size in bytes; -1 means unknown.
	Size int64
}

// ResumableSource is the remote side of [ResumableCopyOption.Pull].
// Implementations may seek, issue HTTP range requests, or discard bytes to
// reach the requested offset.
type ResumableSource interface {
	// Open returns a reader whose first byte is at content offset offset,
	// together with the content's identity and size as known to the source.
	// The caller is responsible for closing the returned ReadCloser.
	Open(ctx context.Context, offset int64) (io.ReadCloser, ContentInfo, error)
}

// SinkState reports what a [ResumableSink] already holds.
type SinkState struct {
	// Offset is the number of bytes of partial content already received.
	Offset int64
	// ETag is the identity token of the partial content; empty if unknown.
	ETag string
	// Complete is true when the content is fully present and committed (the
	// sink has already called Commit or the file was already finalised from a
	// previous run).
	Complete bool
}

// ResumableSink is the remote side of [ResumableCopyOption.Push].
type ResumableSink interface {
	// State reports the current sink state so the pusher can decide where to
	// resume (or whether the transfer is already complete).
	State(ctx context.Context) (SinkState, error)

	// Append consumes r, whose first byte corresponds to content offset
	// offset.  offset == 0 signals a fresh start; implementations must
	// truncate or replace any partial state.  info describes the *full*
	// content being pushed; implementations should persist info.ETag so that
	// a subsequent State call can return it.
	Append(ctx context.Context, info ContentInfo, offset int64, r io.Reader) error

	// Commit finalises the content (e.g. atomic rename from ".part" to the
	// final name).  Commit must be idempotent: calling it when the content is
	// already committed must return nil.
	Commit(ctx context.Context) error
}

// ErrContentChanged is returned by [ResumableCopyOption.Pull] when the source
// reports a different ETag than the caller expected and the mismatch cannot be
// resolved by restarting from offset 0 (the remote simply no longer has the
// requested content).  Callers should use [errors.Is] to check for this error.
var ErrContentChanged = errors.New("content changed")

// ErrSizeMismatch is returned when declared and observed sizes disagree in a
// way that indicates a logic error (e.g. the expected size was given by the
// caller but the source reports a different size, or the part file is larger
// than the declared total).  Callers should use [errors.Is] to check for this
// error.
var ErrSizeMismatch = errors.New("size mismatch")

// resumableFile is the minimal file interface required by the copy primitives
// inside this package.  It intentionally omits higher-level helpers (e.g.
// WriteString) to remain satisfiable by thin file wrappers.
type resumableFile interface {
	ReadFile
	WriteFile
	SeekFile
	CloseFile
	NameFile
}

// resumableFsys is the minimal filesystem interface required by
// [ResumableCopyOption.Pull] and [ResumableCopyOption.Push].
type resumableFsys[File resumableFile] interface {
	OpenFileFs[File]
	StatFs
	RenameFs
	RemoveFs
}

// ResumableCopyOption configures the behaviour of [ResumableCopyOption.Pull]
// and [ResumableCopyOption.Push].
//
// The zero value is ready to use with sensible defaults (.part suffix, 256 KiB
// copy buffer, no pre-commit hooks).
type ResumableCopyOption[Fsys resumableFsys[File], File resumableFile] struct {
	// PartSuffix is appended to the destination path to form the
	// in-progress ("part") file path.  Defaults to ".part" when empty.
	// The ETag sidecar is stored at partPath + ".etag".
	PartSuffix string

	// BufSize is the size of the internal copy buffer in bytes.  0 means
	// use the default of 256 KiB.
	BufSize int

	// PreCommitHooks are called after all bytes have been written to the part
	// file but before the atomic rename to the final path.  The part file is
	// seeked to offset 0 before each hook call; path is the part-file path.
	//
	// Hook invariants (same as [SafeWriteOption]):
	//   - must not Close the file.
	//   - must not Rename the file.
	//   - return a non-nil error to abort; on abort the part file and sidecar
	//     are removed so the next attempt restarts clean.
	//
	// A sha256 integrity check is a natural use for this hook:
	//
	//	func sha256Hook(expected string) func(f *os.File, path string) error {
	//	    return func(f *os.File, path string) error {
	//	        h := sha256.New()
	//	        if _, err := io.Copy(h, f); err != nil {
	//	            return err
	//	        }
	//	        if got := hex.EncodeToString(h.Sum(nil)); got != expected {
	//	            return fmt.Errorf("sha256 mismatch: got %s", got)
	//	        }
	//	        return nil
	//	    }
	//	}
	PreCommitHooks []func(f File, path string) error
}

func (opt ResumableCopyOption[Fsys, File]) partSuffix() string {
	if opt.PartSuffix != "" {
		return opt.PartSuffix
	}
	return defaultPartSuffix
}

func (opt ResumableCopyOption[Fsys, File]) bufSize() int {
	if opt.BufSize > 0 {
		return opt.BufSize
	}
	return 256 * 1024
}

// Shared part/sidecar machinery.
//
// Pull (the local side of a download) and [FsSink] (the local side of an
// upload) must keep their .part + ".part.etag" sidecar handling semantically
// symmetric for resume interoperability.  The naming, sidecar read/write, and
// part-file removal therefore live here as a single unexported helper set used
// by both, rather than being implemented twice.
//
// The helpers are generic over deliberately narrow constraints
// (partSidecarFsys / partSidecarFile) so any caller whose fsys/file satisfies
// them — including [FsSink]'s wider fsSinkFsys/fsSinkFile — can reuse them.

// defaultPartSuffix is the .part suffix used when none is configured.
const defaultPartSuffix = ".part"

// sidecarPerm is the file-creation mode for the ETag sidecar file.
const sidecarPerm fs.FileMode = 0o644

// partSidecarFile is the minimal file interface required by the shared
// sidecar helpers: read for readSidecar, write for writeSidecar, close for
// both.
type partSidecarFile interface {
	ReadFile
	WriteFile
	CloseFile
}

// partSidecarFsys is the minimal filesystem interface required by the shared
// part/sidecar helpers.
type partSidecarFsys[File partSidecarFile] interface {
	OpenFileFs[File]
	RemoveFs
}

// partPaths returns the part-file path and its ETag sidecar path for a target
// name and part suffix.  suffix must already be resolved (non-empty); callers
// use [defaultPartSuffix] for the zero case.
func partPaths(name, suffix string) (partPath, sidecarPath string) {
	partPath = name + suffix
	sidecarPath = partPath + ".etag"
	return partPath, sidecarPath
}

// removePartFiles removes the part file and its ETag sidecar, ignoring
// not-exist errors.  It returns the first non-not-exist error encountered.
func removePartFiles[Fsys partSidecarFsys[File], File partSidecarFile](
	fsys Fsys,
	partPath, sidecarPath string,
) error {
	err1 := fsys.Remove(partPath)
	if err1 != nil && !errors.Is(err1, fs.ErrNotExist) {
		return err1
	}
	err2 := fsys.Remove(sidecarPath)
	if err2 != nil && !errors.Is(err2, fs.ErrNotExist) {
		return err2
	}
	return nil
}

// readSidecar reads the ETag sidecar at sidecarPath, returning ("", nil) when
// the file does not exist.
func readSidecar[Fsys partSidecarFsys[File], File partSidecarFile](
	fsys Fsys,
	sidecarPath string,
) (string, error) {
	f, err := fsys.OpenFile(sidecarPath, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// writeSidecar writes etag to sidecarPath, truncating any previous content.
func writeSidecar[Fsys partSidecarFsys[File], File partSidecarFile](
	fsys Fsys,
	sidecarPath, etag string,
) error {
	f, err := fsys.OpenFile(sidecarPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, sidecarPerm)
	if err != nil {
		return err
	}
	_, werr := io.WriteString(f, etag)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// ctxCopy copies from r to w using buf, checking ctx cancellation between each
// read-write cycle.  It does NOT wrap context errors.
func ctxCopy(ctx context.Context, w io.Writer, r io.Reader, buf []byte) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		nr, readErr := r.Read(buf)
		if nr > 0 {
			nw, writeErr := w.Write(buf[:nr])
			total += int64(nw)
			if writeErr != nil {
				return total, writeErr
			}
			if nw != nr {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return total, nil
			}
			return total, readErr
		}
	}
}

// resumeDecision is the result of [decideResume]: where a Pull should start
// writing and whether the existing part file must be discarded first.
type resumeDecision struct {
	// startAt is the content offset to resume from: the existing part size
	// when the part is trusted, or 0 when it must be re-fetched from scratch.
	startAt int64
	// discardPart is true when the existing part file (and its sidecar) must
	// be removed before (re)starting, because it cannot be trusted.
	discardPart bool
}

// decideResume is the pure, correctness-critical core of Pull's resume logic:
// given the size of an existing part file, the ETag recorded in its sidecar,
// the caller's expectation, and the source's observed identity, it decides
// whether that part can be appended to (resume at partSize) or must be thrown
// away and re-fetched from offset 0.
//
// It is a pure function (no I/O) so the whole trust matrix is unit-testable
// without driving a real Pull. A wrong "resume" answer means silent corruption,
// so the rules are explicit:
//
//   - Discard when the part is oversized: the caller declared a total size and
//     the part already exceeds it.
//   - Discard when the sidecar ETag conflicts with the caller's expected ETag
//     (both known and different) — the part belongs to different content.
//   - Discard when the source's ETag is known and contradicts what we hold: it
//     matches neither the sidecar ETag nor (when the sidecar is absent) the
//     caller's expected ETag.
//   - Otherwise resume at partSize. A part can be trusted when the source
//     advertises no ETag (size-based resume), the source ETag matches the
//     sidecar, or the sidecar is absent but the source ETag matches what the
//     caller expected.
//
// Passing the zero [ContentInfo] for src (unknown ETag, unknown size) yields
// the pre-open guess Pull uses to choose the offset to open the source at:
// the source-ETag trust check trivially passes, so only the oversize and
// sidecar-conflict checks apply. This is what lets Pull open at the guessed
// offset once and reopen at 0 only when the post-open source identity proves
// the guess wrong — never an extra source round-trip.
func decideResume(
	partSize int64,
	storedETag string,
	expected ContentInfo,
	src ContentInfo,
) resumeDecision {
	if partSize <= 0 {
		return resumeDecision{startAt: 0, discardPart: false}
	}

	// Oversized partial: more bytes than the declared total can exist only if
	// the part is stale.
	if expected.Size >= 0 && partSize > expected.Size {
		return resumeDecision{startAt: 0, discardPart: true}
	}

	// Sidecar ETag conflicts with what the caller expects.
	if storedETag != "" && expected.ETag != "" && storedETag != expected.ETag {
		return resumeDecision{startAt: 0, discardPart: true}
	}

	// Source identity trust check.
	trusted := src.ETag == "" || // no-etag world: size-based resume
		src.ETag == storedETag || // exact sidecar match
		(storedETag == "" && expected.ETag != "" && src.ETag == expected.ETag)
	if !trusted {
		return resumeDecision{startAt: 0, discardPart: true}
	}

	return resumeDecision{startAt: partSize, discardPart: false}
}

// Pull downloads content described by expected from src into a file at name
// inside fsys, using .part + sidecar + atomic-rename to support resuming
// interrupted transfers.
//
// Behaviour summary:
//   - If the final file already exists: return nil when sizes match (or
//     expected.Size < 0); return [ErrSizeMismatch] when sizes differ.
//   - A pre-existing part file is resumed unless it is oversized or its sidecar
//     ETag conflicts with expected.ETag.
//   - PreCommitHooks errors are returned as-is (not wrapped in [*fs.PathError]),
//     so callers can match them with errors.Is/errors.As directly.
//   - If the source returns an ETag that differs from expected.ETag, Pull
//     returns [ErrContentChanged] (the remote no longer has what you asked for;
//     retrying will not help unless the source is fixed).
//   - Mid-copy I/O errors or context cancellation leave the part file in place
//     for a later resume attempt; only pre-commit hook failures remove the part
//     file.
//   - Parent directories must exist before calling Pull.
//   - perm is the file-creation mode for the part file (and therefore the
//     final file after rename).
func (opt ResumableCopyOption[Fsys, File]) Pull(
	ctx context.Context,
	fsys Fsys,
	name string,
	src ResumableSource,
	expected ContentInfo,
	perm fs.FileMode,
) error {
	partPath, sidecarPath := partPaths(name, opt.partSuffix())

	// Step 1: check whether the final file already exists.
	if fi, err := fsys.Stat(name); err == nil {
		if expected.Size >= 0 && fi.Size() != expected.Size {
			return WrapPathErr("pull", name, ErrSizeMismatch)
		}
		return nil // already complete
	} else if !errors.Is(err, fs.ErrNotExist) {
		return WrapPathErr("stat", name, err)
	}

	// Step 2: determine resume point.
	var (
		startAt    int64
		partSize   int64
		storedETag string
	)

	partFi, partErr := fsys.Stat(partPath)
	if partErr == nil {
		partSize = partFi.Size()
		var sideErr error
		storedETag, sideErr = readSidecar[Fsys, File](fsys, sidecarPath)
		if sideErr != nil {
			// An unreadable (as opposed to absent) sidecar must not silently
			// degrade to an unguarded size-based resume.
			return WrapPathErr("read-sidecar", sidecarPath, sideErr)
		}

		// Pre-open guess: decide using only what is knowable before the source
		// is contacted (the zero ContentInfo makes the source-ETag trust check
		// pass trivially, so only oversize and sidecar-conflict can discard).
		guess := decideResume(partSize, storedETag, expected, ContentInfo{Size: -1})
		if guess.discardPart {
			if rerr := removePartFiles[Fsys, File](fsys, partPath, sidecarPath); rerr != nil {
				return WrapPathErr("remove-part", partPath, rerr)
			}
			partSize = 0
			storedETag = ""
		}
		startAt = guess.startAt
	} else if !errors.Is(partErr, fs.ErrNotExist) {
		return WrapPathErr("stat", partPath, partErr)
	}

	// Step 3: open source at the guessed offset.
	r, srcInfo, err := src.Open(ctx, startAt)
	if err != nil {
		return WrapPathErr("open-source", name, err)
	}

	// ETag sanity check: source disagrees with what the caller wants.
	if expected.ETag != "" && srcInfo.ETag != "" && srcInfo.ETag != expected.ETag {
		_ = r.Close()
		return WrapPathErr("pull", name, ErrContentChanged)
	}

	// Authoritative decision now that the source identity is known. When we
	// guessed a resume (startAt > 0) but the source identity does not back it
	// up, discard the part and reopen at 0 (the only extra round-trip, and only
	// on a proven-wrong guess).
	if startAt > 0 {
		decision := decideResume(partSize, storedETag, expected, srcInfo)
		if decision.discardPart {
			_ = r.Close()
			if rerr := removePartFiles[Fsys, File](fsys, partPath, sidecarPath); rerr != nil {
				return WrapPathErr("remove-part", partPath, rerr)
			}
			storedETag = ""
			r, srcInfo, err = src.Open(ctx, 0)
			if err != nil {
				return WrapPathErr("open-source", name, err)
			}
			startAt = 0
		}
	}
	defer r.Close()

	// Resolve effective size (both must agree when both are known).
	effectiveSize := int64(-1)
	switch {
	case srcInfo.Size >= 0 && expected.Size >= 0:
		if srcInfo.Size != expected.Size {
			return WrapPathErr("pull", name, ErrSizeMismatch)
		}
		effectiveSize = srcInfo.Size
	case srcInfo.Size >= 0:
		effectiveSize = srcInfo.Size
	case expected.Size >= 0:
		effectiveSize = expected.Size
	}

	// Resolve effective ETag.
	effectiveETag := srcInfo.ETag
	if effectiveETag == "" {
		effectiveETag = expected.ETag
	}

	// Step 4: open part file; write sidecar if needed.
	// O_RDWR is required so that PreCommitHooks can read back the written data.
	// A fresh start truncates so no stale bytes can survive past `written`.
	partFlag := os.O_RDWR | os.O_CREATE
	if startAt == 0 {
		partFlag |= os.O_TRUNC
	}
	partFile, err := fsys.OpenFile(partPath, partFlag, perm)
	if err != nil {
		return WrapPathErr("open-part", partPath, err)
	}

	// Write/update sidecar BEFORE copying any data.
	if effectiveETag != "" && effectiveETag != storedETag {
		if serr := writeSidecar[Fsys, File](fsys, sidecarPath, effectiveETag); serr != nil {
			_ = partFile.Close()
			return WrapPathErr("write-sidecar", sidecarPath, serr)
		}
	}

	if startAt > 0 {
		if _, serr := partFile.Seek(startAt, io.SeekStart); serr != nil {
			_ = partFile.Close()
			return WrapPathErr("seek-part", partPath, serr)
		}
	}

	// Step 5: copy loop.
	buf := make([]byte, opt.bufSize())
	written, copyErr := ctxCopy(ctx, partFile, r, buf)

	// Step 6: check for short read when size is known.
	if copyErr == nil && effectiveSize >= 0 && startAt+written != effectiveSize {
		// Keep part for later resume.
		_ = partFile.Close()
		return WrapPathErr("pull", name, io.ErrUnexpectedEOF)
	}
	if copyErr != nil {
		// Mid-copy error: keep part for later resume.
		_ = partFile.Close()
		return WrapPathErr("pull", name, copyErr)
	}

	// Step 7: pre-commit hooks.
	for _, hook := range opt.PreCommitHooks {
		if _, serr := partFile.Seek(0, io.SeekStart); serr != nil {
			_ = partFile.Close()
			_ = removePartFiles[Fsys, File](fsys, partPath, sidecarPath)
			return WrapPathErr("pre-commit-seek", partPath, serr)
		}
		if herr := hook(partFile, partPath); herr != nil {
			_ = partFile.Close()
			_ = removePartFiles[Fsys, File](fsys, partPath, sidecarPath)
			return herr
		}
	}

	// Step 8: close, rename, remove sidecar.
	if cerr := partFile.Close(); cerr != nil {
		return WrapPathErr("close-part", partPath, cerr)
	}
	if rerr := fsys.Rename(partPath, name); rerr != nil {
		return WrapLinkErr("rename", partPath, name, rerr)
	}
	// Sidecar no longer needed; not-exist is acceptable.
	_ = fsys.Remove(sidecarPath)
	return nil
}

// Push uploads the file at name inside fsys to sink, supporting resume when
// the sink already holds a partial copy.
//
// Behaviour summary:
//   - If info.Size >= 0 and the local file has a different size, Push returns
//     [ErrSizeMismatch] before contacting the sink.
//   - If sink.State reports Complete == true, Push returns nil immediately.
//   - If the sink's ETag conflicts with info.ETag (both non-empty and
//     different), the resume offset is reset to 0 so the transfer starts over.
//   - If the sink already holds exactly the right number of bytes, Push calls
//     Commit without re-sending data.
//   - Parent directories of name must exist before calling Push.
func (opt ResumableCopyOption[Fsys, File]) Push(
	ctx context.Context,
	fsys Fsys,
	name string,
	sink ResumableSink,
	info ContentInfo,
) error {
	// Step 1: open local file; stat for actual size.
	f, err := fsys.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return WrapPathErr("open", name, err)
	}
	defer f.Close()

	fi, err := fsys.Stat(name)
	if err != nil {
		return WrapPathErr("stat", name, err)
	}
	actualSize := fi.Size()

	if info.Size >= 0 && info.Size != actualSize {
		return WrapPathErr("push", name, ErrSizeMismatch)
	}
	info.Size = actualSize

	// Step 2: query sink state.
	st, err := sink.State(ctx)
	if err != nil {
		return WrapPathErr("sink-state", name, err)
	}
	if st.Complete {
		return nil
	}

	// Step 3: determine resume offset.
	offset := st.Offset
	if offset > actualSize {
		offset = 0
	}
	if st.ETag != "" && info.ETag != "" && st.ETag != info.ETag {
		offset = 0
	}

	// Step 4: sink already has all bytes — just commit.
	if offset == actualSize {
		return sink.Commit(ctx)
	}

	// Step 5: seek to offset and stream.
	if offset > 0 {
		if _, serr := f.Seek(offset, io.SeekStart); serr != nil {
			return WrapPathErr("seek", name, serr)
		}
	}

	lr := io.LimitReader(f, actualSize-offset)
	ctxR := &ctxReader{ctx: ctx, r: lr}
	if aerr := sink.Append(ctx, info, offset, ctxR); aerr != nil {
		return WrapPathErr("push-append", name, aerr)
	}

	// Step 6: commit.
	return sink.Commit(ctx)
}

// ctxReader wraps a reader and checks ctx cancellation before each Read call.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}
