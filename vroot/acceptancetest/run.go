package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// RunFs runs the full set of [vroot.Fs] acceptance tests against an implementation.
//
// Each method test is dispatched via t.Run so callers can filter sub-tests with -run.
// Race tests are placed at the end so non-race failures surface earlier in the log.
func RunFs[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	t.Run("Chmod", func(t *testing.T) { TestChmod(t, s) })
	t.Run("Chown", func(t *testing.T) { TestChown(t, s) })
	t.Run("Chtimes", func(t *testing.T) { TestChtimes(t, s) })
	t.Run("Close", func(t *testing.T) { TestClose(t, s) })
	t.Run("Create", func(t *testing.T) { TestCreate(t, s) })
	t.Run("Lchown", func(t *testing.T) { TestLchown(t, s) })
	t.Run("Link", func(t *testing.T) { TestLink(t, s) })
	t.Run("Lstat", func(t *testing.T) { TestLstat(t, s) })
	t.Run("Mkdir", func(t *testing.T) { TestMkdir(t, s) })
	t.Run("MkdirAll", func(t *testing.T) { TestMkdirAll(t, s) })
	t.Run("Name", func(t *testing.T) { TestName(t, s) })
	t.Run("Open", func(t *testing.T) { TestOpen(t, s) })
	t.Run("OpenFile", func(t *testing.T) { TestOpenFile(t, s) })
	t.Run("ReadLink", func(t *testing.T) { TestReadLink(t, s) })
	t.Run("Remove", func(t *testing.T) { TestRemove(t, s) })
	t.Run("RemoveAll", func(t *testing.T) { TestRemoveAll(t, s) })
	t.Run("Rename", func(t *testing.T) { TestRename(t, s) })
	t.Run("Stat", func(t *testing.T) { TestStat(t, s) })
	t.Run("Symlink", func(t *testing.T) { TestSymlink(t, s) })

	t.Run("File/Chmod", func(t *testing.T) { TestFileChmod(t, s) })
	t.Run("File/Chown", func(t *testing.T) { TestFileChown(t, s) })
	t.Run("File/Close", func(t *testing.T) { TestFileClose(t, s) })
	t.Run("File/Fd", func(t *testing.T) { TestFileFd(t, s) })
	t.Run("File/Name", func(t *testing.T) { TestFileName(t, s) })
	t.Run("File/Read", func(t *testing.T) { TestFileRead(t, s) })
	t.Run("File/ReadAt", func(t *testing.T) { TestFileReadAt(t, s) })
	t.Run("File/ReadDir", func(t *testing.T) { TestFileReadDir(t, s) })
	t.Run("File/Readdir", func(t *testing.T) { TestFileReaddir(t, s) })
	t.Run("File/Readdirnames", func(t *testing.T) { TestFileReaddirnames(t, s) })
	t.Run("File/Seek", func(t *testing.T) { TestFileSeek(t, s) })
	t.Run("File/Stat", func(t *testing.T) { TestFileStat(t, s) })
	t.Run("File/Sync", func(t *testing.T) { TestFileSync(t, s) })
	t.Run("File/Truncate", func(t *testing.T) { TestFileTruncate(t, s) })
	t.Run("File/Write", func(t *testing.T) { TestFileWrite(t, s) })
	t.Run("File/WriteAt", func(t *testing.T) { TestFileWriteAt(t, s) })
	t.Run("File/WriteString", func(t *testing.T) { TestFileWriteString(t, s) })

	t.Run("Race/ReadWrite", func(t *testing.T) { TestRaceReadWrite(t, s) })
	t.Run("Race/Methods", func(t *testing.T) { TestRaceMethods(t, s) })
}

// RunRoot runs the full set of [vroot.Root] acceptance tests, including the Fs subset.
func RunRoot[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) {
	RunFs(t, Setup[F, R]{Make: s.Make, Option: s.Option})
	t.Run("OpenRoot", func(t *testing.T) { TestOpenRoot(t, s) })
	t.Run("Escapes", func(t *testing.T) { TestRootEscapes(t, s) })
}
