package fsutil

import (
	"io"
	"io/fs"
	"path/filepath"

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
	RemoveFs
}

func SafeWrite[File safeWriteFile](fsys safeWriteFsys[File], name string, r io.Reader, perm fs.FileMode) error {
	dir := filepath.Dir(name)

	randomFile, err := OpenFileRandom(fsys, dir, "*.tmp", perm.Perm())
	if err != nil {
		return err
	}

	randomFileName := filepath.Join(dir, filepath.Base(randomFile.Name()))
	defer func() {
		_ = randomFile.Close()
		if err != nil {
			fsys.Remove(randomFileName)
		}
	}()

	bufP := bufpool.GetBytes()
	defer bufpool.PutBytes(bufP)

	buf := *bufP
	_, err = io.CopyBuffer(randomFile, r, buf)
	if err != nil {
		return err
	}

	err = randomFile.Sync()
	if err != nil {
		return err
	}

	err = fsys.Rename(randomFileName, filepath.Clean(name))
	if err != nil {
		return err
	}

	return nil
}
