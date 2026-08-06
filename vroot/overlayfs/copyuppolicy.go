package overlayfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// ErrTypeNotSupported is returned (wrapped) by a [CopyUpPolicy] when the entry
// to copy up is neither a regular file, a directory, nor a symlink. Callers
// test for it with errors.Is.
var ErrTypeNotSupported = errors.New("type not supported")

// DefaultWorkDir is the work directory [CopyUpPolicyStage] uses when none is
// supplied to [NewCopyUpPolicyStage]. It is an explicit, VISIBLE directory at
// the top root; the overlay hides nothing.
const DefaultWorkDir = ".vroot-overlayfs.work"

// defaultNameMax is the fallback NAME_MAX (bytes) for a single path component
// when a policy is constructed with nameMax <= 0. 255 matches most filesystems.
const defaultNameMax = 255

// copyupTempSuffix is the distinctive suffix every copy-up temp file carries.
// A leftover with this suffix is recognizably a crashed copy-up scratch file,
// which makes crash cleanup (self-sweep) safe and bounds collision with user
// ".tmp" files.
const copyupTempSuffix = ".copyup.tmp"

// copyupSuffixReserve is the number of bytes the temp suffix occupies on top of
// the (possibly truncated) base name:
//
//	"<base>" + "." + <10 random digits> + ".copyup.tmp"
//
// i.e. 1 (dot before random) + 10 (random) + len(".copyup.tmp") == 22. The
// base is truncated to nameMax-copyupSuffixReserve so the whole component fits
// within nameMax bytes.
const copyupSuffixReserve = 1 + 10 + len(copyupTempSuffix)

// CopyUpPolicy copies a file, directory, or symlink from a lower layer up to
// the top of an overlay.
type CopyUpPolicy interface {
	// CopyUp copies the file/dir/symlink at name from a lower layer (from) up to
	// the top (to), publishing it at name (a full top-relative path).
	//
	// The entry type is dispatched on from.Lstat(name).Mode():
	//   - regular file: content + mode + best-effort mtime are copied, published
	//     atomically via a temp + Rename within to.
	//   - directory: the single directory is created at name in to with the
	//     source perm and best-effort mtime. Children are NOT copied — the
	//     overlay materializes parents itself and copies children lazily; a
	//     directory copy-up is not opaque (lowers stay visible).
	//   - symlink: the target is read via from.ReadLink and recreated via
	//     to.Symlink.
	//
	// Any other type yields an error satisfying errors.Is(err,
	// ErrTypeNotSupported).
	CopyUp(from, to vroot.Fs[vroot.File], name string) error
}

// copyUp dispatches on the source mode and copies a single entry from→to.
// Regular files are published atomically by writing a temp into tempDir (a
// top-relative directory) using tempPattern as the OpenFileRandom pattern, then
// renaming it onto name within to. Directories and symlinks are created
// directly (those ops are already atomic and need no temp). It is the shared
// engine behind both built-in policies; they differ only in the tempDir /
// tempPattern they compute.
func copyUp(
	from, to vroot.Fs[vroot.File],
	name string,
	tempDir, tempPattern string,
) error {
	info, err := from.Lstat(name)
	if err != nil {
		return fmt.Errorf("copy-up: lstat source %q: %w", name, err)
	}
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		return copyRegularFile(from, to, name, info, tempDir, tempPattern)
	case mode.IsDir():
		return copyDir(from, to, name, info)
	case mode&os.ModeSymlink != 0:
		return copySymlink(from, to, name)
	default:
		return fmt.Errorf("copy-up: %q (mode %s): %w", name, mode, ErrTypeNotSupported)
	}
}

// copyRegularFile copies the regular file at name from→to, publishing it
// atomically: content is written to a randomly named temp under tempDir (via
// fsutil.OpenFileRandom with tempPattern), its mode + best-effort mtime are
// set, it is synced and closed, then renamed onto name within the single to Fs
// (one rename → atomic). On any error before the rename the temp is removed.
func copyRegularFile(
	from, to vroot.Fs[vroot.File],
	name string,
	srcInfo fs.FileInfo,
	tempDir, tempPattern string,
) (err error) {
	src, err := from.Open(name)
	if err != nil {
		return fmt.Errorf("copy-up: open source %q: %w", name, err)
	}
	defer func() { _ = src.Close() }()

	tempFile, err := fsutil.OpenFileRandom[vroot.Fs[vroot.File], vroot.File](
		to, tempDir, tempPattern, srcInfo.Mode().Perm(),
	)
	if err != nil {
		return fmt.Errorf("copy-up: create temp in %q: %w", tempDir, err)
	}
	tempName := path.Join(tempDir, path.Base(tempFile.Name()))

	// published reports whether the temp has been renamed onto name. Until then
	// the temp is ours to clean up on any failure.
	var published bool
	defer func() {
		_ = tempFile.Close()
		if !published {
			_ = to.Remove(tempName)
		}
	}()

	if _, err = io.Copy(tempFile, src); err != nil {
		return fmt.Errorf("copy-up: copy contents of %q: %w", name, err)
	}
	if err = tempFile.Chmod(srcInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("copy-up: chmod temp for %q: %w", name, err)
	}
	if err = tempFile.Sync(); err != nil {
		return fmt.Errorf("copy-up: sync temp for %q: %w", name, err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("copy-up: close temp for %q: %w", name, err)
	}

	// Best-effort mtime; failure here must not fail the copy-up.
	modTime := srcInfo.ModTime()
	_ = to.Chtimes(tempName, modTime, modTime)

	if err = to.Rename(tempName, name); err != nil {
		return fmt.Errorf("copy-up: publish %q: %w", name, err)
	}
	published = true
	return nil
}

// copyDir materializes the single directory at name in to with the source
// perm and best-effort mtime. It does NOT copy children (the overlay does that
// lazily) and does NOT mark the directory opaque.
func copyDir(
	from, to vroot.Fs[vroot.File],
	name string,
	srcInfo fs.FileInfo,
) error {
	_ = from // children are not consulted; from is taken for signature symmetry.
	if err := to.Mkdir(name, srcInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("copy-up: mkdir %q: %w", name, err)
	}
	// Best-effort mtime.
	modTime := srcInfo.ModTime()
	_ = to.Chtimes(name, modTime, modTime)
	return nil
}

// copySymlink reads the symlink target from from and recreates it in to. A
// symlink create is already atomic, so no temp is needed.
func copySymlink(from, to vroot.Fs[vroot.File], name string) error {
	target, err := from.ReadLink(name)
	if err != nil {
		return fmt.Errorf("copy-up: read symlink %q: %w", name, err)
	}
	if err := to.Symlink(target, name); err != nil {
		return fmt.Errorf("copy-up: symlink %q -> %q: %w", name, target, err)
	}
	return nil
}

// copyupTempPattern builds the OpenFileRandom pattern for a copy-up temp whose
// base is derived from name, truncated (at a rune boundary) so the full
// component "<trunc-base>.<10 random>.copyup.tmp" fits within nameMax bytes.
// The single "*" becomes the 10 random digits.
func copyupTempPattern(name string, nameMax int) string {
	if nameMax <= 0 {
		nameMax = defaultNameMax
	}
	base := path.Base(name)
	trunc := fsutil.TruncateUTF8(base, nameMax-copyupSuffixReserve)
	return trunc + ".*" + copyupTempSuffix
}
