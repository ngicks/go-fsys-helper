package synthfs

import (
	"io/fs"
	"path/filepath"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// AddFile adds a file view to the filesystem at the specified path.
// If the parent directory doesn't exist, it will be created with the specified permissions.
// The file will be added with the specified permissions in its metadata.
func (f *fsys) AddFile(name string, view FileView, dirPerm, filePerm fs.FileMode) error {
	dirName := filepath.Dir(name)
	baseName := filepath.Base(name)

	if dirName != "." {
		if err := f.MkdirAll(dirName, dirPerm); err != nil {
			return err
		}
	}

	// Find parent directory
	parentDirent, err := f.root.findDirent(dirName, false, f)
	if err != nil {
		return err
	}

	parentDir, ok := parentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("addfile", name, syscall.ENOTDIR)
	}

	// Check if parent is writable
	if !parentDir.s.isWritable() {
		return fsutil.WrapPathErr("addfile", name, syscall.EACCES)
	}

	// Create the file entry
	newFile := &file{
		metadata: metadata{
			s: stat{
				mode:    filePerm &^ f.umask,
				modTime: f.clock.Now(),
				name:    baseName,
			},
		},
		view: view,
	}

	// Get size from view if possible
	if info, err := statFileView(view); err == nil {
		newFile.s.size = info.Size()
	}

	parentDir.mu.Lock()
	defer parentDir.mu.Unlock()

	// Check if already exists
	if _, ok := parentDir.files[baseName]; ok {
		return fsutil.WrapPathErr("addfile", name, fs.ErrExist)
	}

	parentDir.addEntry(baseName, newFile)
	return nil
}

// AddFs adds all files from the given vroot.Fs to the filesystem under the specified root directory.
// If the root directory doesn't exist, it will be created with the specified permissions.
// All directories and files will be created with the specified permissions.
func (f *fsys) AddFs(root string, vrootFs vroot.Fs, perm fs.FileMode) error {
	root = filepath.Clean(root)

	// Create root directory if needed
	if root != "." {
		if err := f.MkdirAll(root, perm); err != nil {
			return err
		}
	}

	// Walk the filesystem and add all files
	return vroot.WalkDir(vrootFs, ".", &vroot.WalkOption{ResolveSymlink: false}, func(p, realPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate target path
		targetPath := p
		if root != "." && root != "" {
			targetPath = filepath.Join(root, p)
		}

		if info.IsDir() {
			// Skip root directory itself
			if p == "." {
				return nil
			}
			// Create directory
			return f.Mkdir(targetPath, perm)
		}

		// Create file view and add it
		view, err := NewVrootFsFileView(vrootFs, p)
		if err != nil {
			return err
		}

		// Get the actual file permissions from the source
		filePerm := info.Mode().Perm()
		// Ensure at least read permission
		if filePerm&0o400 == 0 {
			filePerm |= 0o400
		}

		return f.AddFile(targetPath, view, perm, filePerm)
	})
}
