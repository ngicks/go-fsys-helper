package fsutil

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/bufpool"
)

type safeWriteFile interface {
	WriteFile
	CloseFile
	NameFile
	SyncFile
}

type safeWriteFsys[File safeWriteFile] interface {
	OpenFileFs[File]
	RenameFs
	RemoveAllFs
	ChmodFs
	MkdirFs
}

// SyncHook syncs the file to ensure data is written to disk.
//
// From what you can read in linux man page for close(2): https://man7.org/linux/man-pages/man2/close.2.html#NOTES
// you may want to set IgnoreCloseErr option in [SafeWriteOption] to true and put this hook to PostHooks.
func SyncHook[File safeWriteFile](f File, path string) error {
	return f.Sync()
}

// SafeWriteOption configures safe write operations.
type SafeWriteOption[Fsys safeWriteFsys[File], File safeWriteFile] struct {
	// TempFilePolicy is used to create temporary files.
	// If nil, TempFilePolicyRandom will be used.
	TempFilePolicy TempFilePolicy[Fsys, File]
	CopyFsOption   CopyFsOption[Fsys, File]
	// PreHooks and PostHooks are functions called before and after actually copying.
	//
	// Hook invriatns:
	//  - should not Close the file.
	//  - must not Rename the file to other name. Doing so may cause undefined behvaior: concurrent safe-write fails or wrong files are wirtten to final destination.
	//  - must not Remove the file. Instead just return a non-nil error.
	PreHooks, PostHooks []func(f File, path string) error
	// If true, Copy ignores error returned when closing temporary file.
	// Useful if used with SyncHook.
	IgnoreCloseErr bool
}

// Write performs safe write using the provided writer function.
func (opt SafeWriteOption[Fsys, File]) Write(
	fsys Fsys,
	name string,
	writeFunc func(w io.Writer) error,
	perm fs.FileMode,
	preHooks, postHooks []func(f File, path string) error,
) error {
	policy := opt.TempFilePolicy
	if policy == nil {
		policy = TempFilePolicyRandom[Fsys, File]{}
	}

	tempFile, tempDir, err := policy.Create(fsys, name, perm)
	if err != nil {
		return err
	}

	return opt.safeOperation(fsys, name, tempFile, tempDir, preHooks, postHooks, func(file File) error {
		return writeFunc(file)
	})
}

// Copy performs safe copy from a reader using the provided options.
func (opt SafeWriteOption[Fsys, File]) Copy(
	fsys Fsys,
	name string,
	r io.Reader,
	perm fs.FileMode,
	preHooks, postHooks []func(f File, path string) error,
) error {
	policy := opt.TempFilePolicy
	if policy == nil {
		policy = TempFilePolicyRandom[Fsys, File]{}
	}

	tempFile, tempDir, err := policy.Create(fsys, name, perm)
	if err != nil {
		return err
	}

	return opt.safeOperation(fsys, name, tempFile, tempDir, preHooks, postHooks, func(file File) error {
		bufP := bufpool.GetBytes()
		defer bufpool.PutBytes(bufP)

		buf := *bufP
		_, err := io.CopyBuffer(file, r, buf)
		return err
	})
}

// CopyFs performs safe copy from a filesystem using the provided options.
func (opt SafeWriteOption[Fsys, File]) CopyFs(
	fsys Fsys,
	name string,
	src fs.FS,
	perm fs.FileMode,
	preHooks, postHooks []func(f File, path string) error,
) error {
	policy := opt.TempFilePolicy
	if policy == nil {
		policy = TempFilePolicyRandom[Fsys, File]{}
	}

	tempFile, tempDir, err := policy.Mkdir(fsys, name, perm)
	if err != nil {
		return err
	}

	return opt.safeOperation(fsys, name, tempFile, tempDir, preHooks, postHooks, func(file File) error {
		// Use base name from tempFile.Name() to get the temporary directory path
		tempBaseName := filepath.Base(file.Name())
		tempPath := filepath.Join(tempDir, tempBaseName)
		return opt.CopyFsOption.CopyAll(fsys, src, tempPath)
	})
}

// safeOperation performs the common safe operation logic for Copy, Write, and CopyFs.
func (opt SafeWriteOption[Fsys, File]) safeOperation(
	fsys Fsys,
	name string,
	tempFile File,
	tempDir string,
	preHooks, postHooks []func(f File, path string) error,
	operation func(File) error,
) error {
	// Use base name from tempFile.Name() and join with tempDir
	tempBaseName := filepath.Base(tempFile.Name())
	tempPath := filepath.Join(tempDir, tempBaseName)

	closed := false
	var err error

	defer func() {
		if err != nil {
			if !closed {
				_ = tempFile.Close()
			}
			_ = fsys.RemoveAll(tempPath)
		}
	}()

	// Run pre-hooks: default pre hooks first, then argument pre hooks
	for _, hook := range opt.PreHooks {
		if err = hook(tempFile, name); err != nil {
			return err
		}
	}
	for _, hook := range preHooks {
		if err = hook(tempFile, name); err != nil {
			return err
		}
	}

	// Perform the specific operation (write content or create directory)
	if err = operation(tempFile); err != nil {
		return err
	}

	// Run post-hooks: argument post hooks first, then default post hooks
	for _, hook := range postHooks {
		if err = hook(tempFile, name); err != nil {
			return err
		}
	}
	for _, hook := range opt.PostHooks {
		if err = hook(tempFile, name); err != nil {
			return err
		}
	}

	closed = true // at least tried.
	if err = tempFile.Close(); err != nil && !opt.IgnoreCloseErr {
		return err
	}

	err = fsys.Rename(tempPath, filepath.Clean(name))
	if err != nil {
		return err
	}

	return nil
}

// TempFilePolicy manages temporary file creation and cleanup.
type TempFilePolicy[Fsys safeWriteFsys[File], File safeWriteFile] interface {
	// Create creates a temporary file for the given target path.
	// Returns the file and the directory where it was created.
	Create(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error)
	// Mkdir creates a temporary directory for the given target path.
	// Returns the directory file and the directory where it was created.
	Mkdir(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error)
	// WalkFunc processes a single entry during filesystem traversal.
	// It checks if the path matches this policy and removes the file if it matches.
	WalkFunc(fsys Fsys, path string, d fs.DirEntry, err error) error
	// Match returns true if the given path matches the pattern of temporary files created by this policy.
	Match(path string) bool
}

var _ TempFilePolicy[safeWriteFsys[safeWriteFile], safeWriteFile] = (*TempFilePolicyRandom[safeWriteFsys[safeWriteFile], safeWriteFile])(nil)

// TempFilePolicyRandom creates temporary files using random names.
type TempFilePolicyRandom[Fsys safeWriteFsys[File], File safeWriteFile] struct{}

func NewTempFilePolicyRandom[Fsys safeWriteFsys[File], File safeWriteFile]() TempFilePolicyRandom[Fsys, File] {
	return TempFilePolicyRandom[Fsys, File]{}
}

func (p TempFilePolicyRandom[Fsys, File]) pattern(path string) string {
	base := filepath.Base(path)
	const maxPrefix = 255 /* max filename in ext4 */ - 1 - 10 /*random pttern*/ - len(".tmp")
	if len(base) > maxPrefix {
		// truncate base using utf.DedeRuneString
		part := base
		off := 0
		for len(part) > 0 {
			_, n := utf8.DecodeRuneInString(part)
			if off+n > maxPrefix {
				base = base[:off]
				break
			}
			off += n
			part = part[n:]
		}
	}
	return base + ".*.tmp"
}

func (p TempFilePolicyRandom[Fsys, File]) Create(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error) {
	dir := filepath.Dir(filepath.Clean(targetPath))
	file, err := OpenFileRandom(fsys, dir, p.pattern(targetPath), perm.Perm())
	if err != nil {
		return file, "", err
	}
	return file, dir, nil
}

func (p TempFilePolicyRandom[Fsys, File]) Mkdir(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error) {
	dir := filepath.Dir(filepath.Clean(targetPath))
	file, err := MkdirRandom(fsys, dir, p.pattern(targetPath), perm.Perm())
	if err != nil {
		return file, "", err
	}
	return file, dir, nil
}

func (p TempFilePolicyRandom[Fsys, File]) WalkFunc(fsys Fsys, path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	path = filepath.FromSlash(path)

	if !p.Match(path) {
		return nil
	}

	// For directories, remove them and skip their contents
	if d.IsDir() {
		err := fsys.RemoveAll(path)
		if err != nil {
			return err
		}
		return fs.SkipDir
	}

	// Remove files
	return fsys.RemoveAll(path)
}

func (p TempFilePolicyRandom[Fsys, File]) Match(path string) bool {
	base := filepath.Base(path)
	if !strings.HasSuffix(base, ".tmp") {
		return false
	}

	// Remove .tmp extension
	nameWithoutExt := strings.TrimSuffix(base, ".tmp")
	
	// Find the last dot in the name (should separate basename from random digits)
	lastDotIndex := strings.LastIndex(nameWithoutExt, ".")
	if lastDotIndex == -1 || lastDotIndex == 0 {
		return false // no dot or starts with dot (no basename)
	}

	// Check if the part after the last dot is exactly 10 digits
	randomPart := nameWithoutExt[lastDotIndex+1:]
	if len(randomPart) != 10 {
		return false
	}

	// Check if all characters in random part are digits
	for _, char := range randomPart {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

// TempFilePolicyDir creates temporary files in a dedicated directory.
type TempFilePolicyDir[Fsys safeWriteFsys[File], File safeWriteFile] struct {
	tempDir string
}

func NewTempFilePolicyDir[Fsys safeWriteFsys[File], File safeWriteFile](tempDir string) TempFilePolicyDir[Fsys, File] {
	return TempFilePolicyDir[Fsys, File]{
		tempDir: filepath.Clean(tempDir),
	}
}

// Create creates a temporary file in the dedicated directory.
func (p TempFilePolicyDir[Fsys, File]) Create(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error) {
	if err := fsys.Mkdir(p.tempDir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return *new(File), "", err
	}

	file, err := OpenFileRandom(fsys, p.tempDir, "*.tmp", perm.Perm())
	if err != nil {
		return file, "", err
	}
	return file, p.tempDir, nil
}

// Mkdir creates a temporary directory in the dedicated directory.
func (p TempFilePolicyDir[Fsys, File]) Mkdir(fsys Fsys, targetPath string, perm fs.FileMode) (File, string, error) {
	if err := fsys.Mkdir(p.tempDir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return *new(File), "", err
	}

	file, err := MkdirRandom(fsys, p.tempDir, "*.tmp", perm.Perm())
	if err != nil {
		return file, "", err
	}
	return file, p.tempDir, nil
}

// WalkFunc processes temporary files in the dedicated directory during filesystem traversal.
func (p TempFilePolicyDir[Fsys, File]) WalkFunc(fsys Fsys, path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	path = filepath.FromSlash(path)
	cleanPath := filepath.Clean(path)

	// If this is the root temp directory itself, continue
	if cleanPath == p.tempDir {
		return nil
	}

	// If current path is a parent of TempDir (i.e., we haven't reached TempDir yet), continue
	if d.IsDir() {
		var isParent bool
		switch cleanPath {
		case p.tempDir:
			isParent = false // exact match is not a parent relationship
		case ".":
			isParent = true // current directory is parent of any subdirectory
		default:
			isParent = strings.HasPrefix(p.tempDir+string(filepath.Separator), cleanPath+string(filepath.Separator))
		}

		if isParent {
			return nil
		}
	}

	// Check if path matches our pattern
	if !p.Match(path) {
		// For directories that don't match, skip their contents
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}

	// For directories, remove them and skip their contents
	if d.IsDir() {
		err := fsys.RemoveAll(path)
		if err != nil {
			return err
		}
		return fs.SkipDir
	}

	return fsys.RemoveAll(path)
}

// Match returns true if the path is within the temporary directory and matches temp file pattern (10 digits + .tmp).
func (p TempFilePolicyDir[Fsys, File]) Match(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanTempDir := filepath.Clean(p.tempDir)

	// Check if path is within temp directory
	isInTempDir := cleanPath == cleanTempDir ||
		(len(cleanPath) > len(cleanTempDir)+1 &&
			cleanPath[:len(cleanTempDir)] == cleanTempDir &&
			cleanPath[len(cleanTempDir)] == filepath.Separator)

	if !isInTempDir {
		return false
	}

	// Check if filename matches temp file pattern (10 digits + .tmp)
	base := filepath.Base(cleanPath)
	if !strings.HasSuffix(base, ".tmp") {
		return false
	}

	// Remove .tmp extension and check if remaining part is exactly 10 digits
	nameWithoutExt := strings.TrimSuffix(base, ".tmp")
	if len(nameWithoutExt) != 10 {
		return false
	}

	// Check if all characters are digits
	for _, char := range nameWithoutExt {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
