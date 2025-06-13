package overlay

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ CopyPolicy = (*CopyPolicyDotTmp)(nil)

// CopyPolicyDotTmp copies file by creating temporary file on the same directory
// name would be written,
// then calls [io.CopyBuffer] with a temporary file.
// It also copies metadata calling Chmod, Chown, Chtimes, etc for best effort.
// It calls Sync to ensure file is correctly flushed to the backing storage.
// Finally it calls Rename the temporary to name.
type CopyPolicyDotTmp struct {
	// temp file pattern
	pattern string
}

// NewCopyPolicyDotTmp creates a new CopyPolicyDotTmp with the given pattern.
// If pattern is empty, it defaults to ".tmp%d"
func NewCopyPolicyDotTmp(pattern string) *CopyPolicyDotTmp {
	if pattern == "" {
		pattern = "*.tmp"
	}
	return &CopyPolicyDotTmp{pattern: pattern}
}

// CopyTo copies a file from the source layer to the destination vroot.Rooted
func (c *CopyPolicyDotTmp) CopyTo(from Layer, to vroot.Rooted, name string) error {
	info, err := from.Lstat(name)
	if err != nil {
		return err
	}
	switch {
	case info.Mode().IsRegular():
		return c.copyFile(from, to, name)
	case info.Mode().IsDir():
		return c.copyDir(from, to, name)
	case info.Mode()&os.ModeSymlink != 0:
		return c.copySymlink(from, to, name)
	}
	return ErrTypeNotSupported
}

func (c *CopyPolicyDotTmp) copyFile(from Layer, to vroot.Rooted, name string) error {
	// Open source file for reading
	srcFile, err := from.Open(name)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", name, err)
	}
	defer srcFile.Close()

	// Get source file info for metadata
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %s: %w", name, err)
	}

	// Create temporary file in the same directory as the target
	dir := filepath.Dir(name)

	tempFile, err := fsutil.OpenFileRandom(to, dir, "*.tmp", fs.ModePerm)
	if err != nil {
		return fmt.Errorf("opening temp file: %w", err)
	}
	tempName := filepath.Join(dir, filepath.Base(tempFile.Name()))
	// Ensure we clean up the temp file if something goes wrong
	defer func() {
		_ = tempFile.Close()
		if err != nil {
			_ = to.Remove(tempName)
		}
	}()

	// Copy file contents
	_, err = io.Copy(tempFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Copy metadata - file mode
	if err := tempFile.Chmod(srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to copy file mode: %w", err)
	}

	// Sync the file to ensure data is written
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	// Close the temp file before renaming
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Copy timestamps using the filesystem method - best effort
	// Note: This is best-effort as not all filesystems support this
	modTime := srcInfo.ModTime()
	if err := to.Chtimes(tempName, modTime, modTime); err != nil {
		// Don't fail the copy for timestamp errors, just continue
	}

	// Atomically rename temp file to target name
	if err := to.Rename(tempName, name); err != nil {
		return fmt.Errorf("failed to rename temporary file to target: %w", err)
	}

	return nil
}

func (c *CopyPolicyDotTmp) copyDir(from Layer, to vroot.Rooted, name string) error {
	// Find the first directory that doesn't exist
	var firstNonExistent string
	var firstNonExistentFound bool
	for dir := range pathFromHead(name) {
		_, err := to.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			firstNonExistent = dir
			firstNonExistentFound = true
			break
		} else if err != nil {
			return fmt.Errorf("failed to stat directory %s: %w", dir, err)
		}
	}

	// If all directories exist, just apply metadata
	if !firstNonExistentFound {
		for dir := range pathFromHead(name) {
			srcInfo, err := from.Lstat(dir)
			if err != nil {
				return fmt.Errorf("failed to stat source directory %s: %w", dir, err)
			}

			// Copy directory metadata - best effort
			if err := to.Chmod(dir, srcInfo.Mode()); err != nil {
				// Don't fail for chmod errors, just continue
			}

			// Copy timestamps - best effort
			modTime := srcInfo.ModTime()
			if err := to.Chtimes(dir, modTime, modTime); err != nil {
				// Don't fail for timestamp errors, just continue
			}
		}
		return nil
	}

	// Create the first non-existent directory with .tmp suffix
	tmpDir := firstNonExistent + ".tmp"

	// Get source info for the first directory
	srcInfo, err := from.Lstat(firstNonExistent)
	if err != nil {
		return fmt.Errorf("failed to stat source directory %s: %w", firstNonExistent, err)
	}

	perm := srcInfo.Mode().Perm()
	err = to.Mkdir(tmpDir, perm)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory %s: %w", tmpDir, err)
	}

	// Ensure cleanup on failure
	var success bool
	defer func() {
		if !success {
			_ = to.RemoveAll(tmpDir)
		}
	}()

	// Create remaining directories without .tmp suffix
	// We need to create them inside the .tmp directory structure
	var foundFirstNonExistent bool
	for dir := range pathFromHead(name) {
		if !foundFirstNonExistent {
			if dir == firstNonExistent {
				foundFirstNonExistent = true
			}
			continue
		}

		srcInfo, err := from.Lstat(dir)
		if err != nil {
			return fmt.Errorf("failed to stat source directory %s: %w", dir, err)
		}

		// Replace the first non-existent directory part with .tmp version
		targetDir := tmpDir + dir[len(firstNonExistent):]

		perm := srcInfo.Mode().Perm()
		err = to.Mkdir(targetDir, perm)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
		}
	}

	// Apply metadata to all directories (including the .tmp one)
	foundFirstNonExistent = false
	for dir := range pathFromHead(name) {
		var targetDir string
		if !foundFirstNonExistent {
			if dir == firstNonExistent {
				foundFirstNonExistent = true
				targetDir = tmpDir
			} else {
				targetDir = dir
			}
		} else {
			// Replace the first non-existent directory part with .tmp version
			targetDir = tmpDir + dir[len(firstNonExistent):]
		}

		srcInfo, err := from.Lstat(dir)
		if err != nil {
			return fmt.Errorf("failed to stat source directory %s: %w", dir, err)
		}

		// Copy directory metadata - best effort
		if err := to.Chmod(targetDir, srcInfo.Mode()); err != nil {
			// Don't fail for chmod errors, just continue
		}

		// Copy timestamps - best effort
		modTime := srcInfo.ModTime()
		if err := to.Chtimes(targetDir, modTime, modTime); err != nil {
			// Don't fail for timestamp errors, just continue
		}
	}

	// Atomically rename .tmp directory to final name
	err = to.Rename(tmpDir, firstNonExistent)
	if err != nil {
		return fmt.Errorf("failed to rename temporary directory %s to %s: %w", tmpDir, firstNonExistent, err)
	}

	success = true
	return nil
}

func (c *CopyPolicyDotTmp) copySymlink(from Layer, to vroot.Rooted, name string) error {
	// Read the symlink target
	target, err := from.ReadLink(name)
	if err != nil {
		return fmt.Errorf("failed to read symlink %s: %w", name, err)
	}

	// Create the symlink in the destination
	if err := to.Symlink(target, name); err != nil {
		return fmt.Errorf("failed to create symlink %s -> %s: %w", name, target, err)
	}

	return nil
}

func pathFromHead(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		cut := ""
		name := filepath.Clean(name)
		rest := name
		for len(rest) > 0 {
			i := strings.Index(rest, string(filepath.Separator))
			if i < 0 {
				yield(name)
				return
			}
			cut = name[:len(cut)+i]
			if !yield(cut) {
				return
			}
			cut = name[:len(cut)+1] // include last sep
			rest = rest[i+len(string(filepath.Separator)):]
		}
	}
}

func pathFromTail(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		if !yield(name) {
			return
		}
		rest := name
		for len(rest) > 0 {
			i := strings.LastIndex(rest, string(filepath.Separator))
			if i < 0 {
				return
			}
			rest = rest[:i]
			if !yield(rest) {
				return
			}
		}
	}
}
