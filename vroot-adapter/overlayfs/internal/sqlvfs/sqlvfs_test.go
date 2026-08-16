package sqlvfs_test

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/internal/sqlvfs"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

const dbName = "meta.sqlite3"

func newOsfs(t *testing.T, dir string) vroot.Fs[vroot.File] {
	t.Helper()
	fsys, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("osfs.NewFs: %v", err)
	}
	t.Cleanup(func() { _ = fsys.Close() })
	return vroot.Widen[*osfs.File](fsys)
}

type backend struct {
	name string
	open func(t *testing.T) vroot.Fs[vroot.File]
}

// backends lists the fsys implementations the store must work over: osfs, whose
// files lock through vroot.Locker, and memfs, whose files do not.
func backends() []backend {
	return []backend{
		{"memfs", func(t *testing.T) vroot.Fs[vroot.File] { return memfs.New("mem") }},
		{"osfs", func(t *testing.T) vroot.Fs[vroot.File] { return newOsfs(t, t.TempDir()) }},
	}
}

func openDB(t *testing.T, fsys vroot.Fs[vroot.File], pragmas ...string) *sqlvfs.DB {
	t.Helper()
	db, err := sqlvfs.Open(fsys, dbName, pragmas...)
	if err != nil {
		t.Fatalf("sqlvfs.Open: %v", err)
	}
	return db
}

func exists(t *testing.T, fsys vroot.Fs[vroot.File], name string) bool {
	t.Helper()
	_, err := fsys.Stat(name)
	if err == nil {
		return true
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %q: %v", name, err)
	}
	return false
}

// TestRoundTrip writes rows through the VFS, closes the database, reopens it
// from the same fsys and reads them back: the bytes have to survive in the fsys,
// not in the connection.
func TestRoundTrip(t *testing.T) {
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			fsys := b.open(t)

			const ddl = `CREATE TABLE node (path TEXT PRIMARY KEY, kind INTEGER NOT NULL) WITHOUT ROWID`

			db := openDB(t, fsys)
			if _, err := db.Exec(ddl); err != nil {
				t.Fatalf("create table: %v", err)
			}
			for _, row := range []struct {
				path string
				kind int
			}{{"a", 1}, {"a/b", 2}, {"c", 1}} {
				if _, err := db.Exec(
					`INSERT INTO node VALUES (?, ?)`,
					row.path,
					row.kind,
				); err != nil {
					t.Fatalf("insert %q: %v", row.path, err)
				}
			}

			if !exists(t, fsys, dbName) {
				t.Fatalf("%q not created inside the fsys", dbName)
			}

			var kind int
			if err := db.QueryRow(`SELECT kind FROM node WHERE path = ?`, "a/b").
				Scan(&kind); err != nil {
				t.Fatalf("select: %v", err)
			}
			if kind != 2 {
				t.Fatalf("kind = %d, want 2", kind)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}

			reopened := openDB(t, fsys)
			defer reopened.Close()

			rows, err := reopened.Query(`SELECT path, kind FROM node ORDER BY path`)
			if err != nil {
				t.Fatalf("query after reopen: %v", err)
			}
			defer rows.Close()

			var got []string
			for rows.Next() {
				var path string
				var kind int
				if err := rows.Scan(&path, &kind); err != nil {
					t.Fatalf("scan: %v", err)
				}
				got = append(got, path)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
			want := []string{"a", "a/b", "c"}
			if len(got) != len(want) {
				t.Fatalf("paths after reopen = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("paths after reopen = %q, want %q", got, want)
				}
			}
		})
	}
}

// TestJournalLivesInFsys checks that the rollback journal is written through the
// VFS: it can only appear next to the database inside the fsys, and under plain
// locking it is deleted again when the transaction commits.
func TestJournalLivesInFsys(t *testing.T) {
	const journal = dbName + "-journal"

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			fsys := b.open(t)
			// locking_mode=normal is what makes the journal deletion observable;
			// under the store's exclusive mode sqlite keeps the file and zeroes its
			// header instead, which TestJournalPersistsUnderExclusiveLock pins down.
			db := openDB(t, fsys, "locking_mode(normal)")
			defer db.Close()

			if _, err := db.Exec(`CREATE TABLE t (v TEXT)`); err != nil {
				t.Fatalf("create table: %v", err)
			}
			if exists(t, fsys, journal) {
				t.Fatalf("%q present outside a transaction", journal)
			}

			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			if _, err := tx.Exec(`INSERT INTO t VALUES ('x')`); err != nil {
				t.Fatalf("insert: %v", err)
			}
			if !exists(t, fsys, journal) {
				t.Fatalf("%q missing during a write transaction", journal)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			if exists(t, fsys, journal) {
				t.Fatalf("%q still present after commit", journal)
			}
		})
	}
}

// TestJournalPersistsUnderExclusiveLock records the journal lifetime the store
// actually gets: with locking_mode=EXCLUSIVE sqlite keeps the journal file after
// commit rather than deleting it.
func TestJournalPersistsUnderExclusiveLock(t *testing.T) {
	const journal = dbName + "-journal"

	fsys := memfs.New("mem")
	db := openDB(t, fsys)
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE t (v TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if !exists(t, fsys, journal) {
		t.Fatalf("%q missing after a committed write", journal)
	}
}

// TestRegisterUnregister checks the per-instance registration: a generated name
// resolves to the FS while it is registered, and to nothing afterwards.
func TestRegisterUnregister(t *testing.T) {
	fsys := memfs.New("mem")
	f := sqlvfs.Register(fsys)

	if got := vfs.Find(f.Name()); got != vfs.VFS(f) {
		t.Fatalf("vfs.Find(%q) = %v, want the registered FS", f.Name(), got)
	}
	if other := sqlvfs.Register(memfs.New("other")); other.Name() == f.Name() {
		t.Fatalf("two FS instances share the name %q", f.Name())
	} else {
		other.Unregister()
	}

	f.Unregister()
	if got := vfs.Find(f.Name()); got != nil {
		t.Fatalf("vfs.Find(%q) after Unregister = %v, want nil", f.Name(), got)
	}
}

func openVFSFile(t *testing.T, fsys vroot.Fs[vroot.File]) vfs.File {
	t.Helper()
	f := sqlvfs.Register(fsys)
	t.Cleanup(f.Unregister)
	file, _, err := f.Open(dbName, vfs.OPEN_MAIN_DB|vfs.OPEN_READWRITE|vfs.OPEN_CREATE)
	if err != nil {
		t.Fatalf("FS.Open: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func lockable(t *testing.T, fsys vroot.Fs[vroot.File]) bool {
	t.Helper()
	f, err := fsys.OpenFile("probe", os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()
	_, ok := f.(vroot.Locker)
	return ok
}

// TestLockLevelMapping checks the sqlite-to-vroot lock mapping on a backend that
// implements vroot.Locker: SHARED holders coexist, RESERVED takes the exclusive
// lock that keeps a second handle out, and unlocking lets that handle in.
func TestLockLevelMapping(t *testing.T) {
	dir := t.TempDir()
	fsysA := newOsfs(t, dir)
	fsysB := newOsfs(t, dir)
	if !lockable(t, fsysA) {
		t.Skipf("osfs files do not implement vroot.Locker on %s", runtime.GOOS)
	}

	a := openVFSFile(t, fsysA)
	b := openVFSFile(t, fsysB)

	if err := a.Lock(vfs.LOCK_SHARED); err != nil {
		t.Fatalf("a.Lock(SHARED): %v", err)
	}
	if err := b.Lock(vfs.LOCK_SHARED); err != nil {
		t.Fatalf("b.Lock(SHARED) while a holds SHARED: %v", err)
	}
	if err := b.Unlock(vfs.LOCK_NONE); err != nil {
		t.Fatalf("b.Unlock(NONE): %v", err)
	}

	if err := a.Lock(vfs.LOCK_RESERVED); err != nil {
		t.Fatalf("a.Lock(RESERVED): %v", err)
	}
	if reserved, err := a.CheckReservedLock(); err != nil || !reserved {
		t.Fatalf("a.CheckReservedLock() = %v, %v, want true, nil", reserved, err)
	}

	// vroot.Locker has no try/timeout form, so a contended lock blocks; the wait
	// itself is the assertion, and closing a releases it.
	locked := make(chan error, 1)
	go func() { locked <- b.Lock(vfs.LOCK_SHARED) }()

	select {
	case err := <-locked:
		t.Fatalf("b.Lock(SHARED) returned %v while a holds the exclusive lock", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := a.Unlock(vfs.LOCK_NONE); err != nil {
		t.Fatalf("a.Unlock(NONE): %v", err)
	}
	select {
	case err := <-locked:
		if err != nil {
			t.Fatalf("b.Lock(SHARED) after a unlocked: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("b.Lock(SHARED) still blocked after a unlocked")
	}
}

// TestLockBookkeepingWithoutLocker checks the fallback used by backends whose
// files cannot lock: levels are tracked in process, so sqlite still sees a
// consistent lock state machine.
func TestLockBookkeepingWithoutLocker(t *testing.T) {
	fsys := memfs.New("mem")
	if lockable(t, fsys) {
		t.Fatalf("%T files implement vroot.Locker; this test covers the fallback", fsys)
	}

	f := openVFSFile(t, fsys)
	state, ok := f.(vfs.FileLockState)
	if !ok {
		t.Fatalf("%T does not implement vfs.FileLockState", f)
	}

	for _, level := range []vfs.LockLevel{vfs.LOCK_SHARED, vfs.LOCK_RESERVED, vfs.LOCK_EXCLUSIVE} {
		if err := f.Lock(level); err != nil {
			t.Fatalf("Lock(%d): %v", level, err)
		}
		if got := state.LockState(); got != level {
			t.Fatalf("LockState() = %d, want %d", got, level)
		}
	}
	if reserved, err := f.CheckReservedLock(); err != nil || !reserved {
		t.Fatalf("CheckReservedLock() = %v, %v, want true, nil", reserved, err)
	}

	if err := f.Unlock(vfs.LOCK_SHARED); err != nil {
		t.Fatalf("Unlock(SHARED): %v", err)
	}
	if got := state.LockState(); got != vfs.LOCK_SHARED {
		t.Fatalf("LockState() after downgrade = %d, want %d", got, vfs.LOCK_SHARED)
	}
	if reserved, err := f.CheckReservedLock(); err != nil || reserved {
		t.Fatalf("CheckReservedLock() after downgrade = %v, %v, want false, nil", reserved, err)
	}

	if err := f.Unlock(vfs.LOCK_NONE); err != nil {
		t.Fatalf("Unlock(NONE): %v", err)
	}
	if got := state.LockState(); got != vfs.LOCK_NONE {
		t.Fatalf("LockState() after release = %d, want %d", got, vfs.LOCK_NONE)
	}
}

// TestExclusiveLockingKeepsSecondConnectionOut checks the whole stack under the
// store's own settings: while one connection holds the database in
// locking_mode=EXCLUSIVE a second one over the same directory cannot get in, and
// it proceeds as soon as the first closes.
//
// "Cannot get in" takes the shape the platform gives it. Where the lock is
// advisory the second connection parks in Lock and resumes on release; on
// windows it is refused outright, so it is retried after the release instead.
func TestExclusiveLockingKeepsSecondConnectionOut(t *testing.T) {
	dir := t.TempDir()
	first := newOsfs(t, dir)
	if !lockable(t, first) {
		t.Skipf("osfs files do not implement vroot.Locker on %s", runtime.GOOS)
	}

	db := openDB(t, first)
	if _, err := db.Exec(`CREATE TABLE t (v TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t VALUES ('x')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	second := newOsfs(t, dir)
	// read is one whole second connection over the same directory: open, read the
	// row, close.
	read := func() (string, error) {
		other, err := sqlvfs.Open(second, dbName)
		if err != nil {
			return "", err
		}
		defer other.Close()
		var v string
		err = other.QueryRow(`SELECT v FROM t`).Scan(&v)
		return v, err
	}

	type result struct {
		v   string
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := read()
		done <- result{v: v, err: err}
	}()

	if runtime.GOOS == "windows" {
		// LockFileEx ranges are mandatory and osfs locks the whole file, so the
		// second connection never reaches Lock: sqlite reads the database header
		// before locking anything, that read overlaps the range the first
		// connection holds, and windows fails it with ERROR_LOCK_VIOLATION, which
		// arrives here as a disk I/O error out of Open. Refusing is exclusion too —
		// stricter than the advisory lock elsewhere, since it stops even a reader
		// that takes no lock — so what is asserted is that it fails rather than
		// waits. The error itself is not matched: it is regenerated as SQLITE_IOERR
		// inside sqlite, and the windows errno does not survive that.
		select {
		case got := <-done:
			if got.err == nil {
				t.Fatalf(
					"second connection read %q while the first holds the database",
					got.v,
				)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("second connection waits for the first instead of being refused")
		}

		if err := db.Close(); err != nil {
			t.Fatalf("close first: %v", err)
		}
		v, err := read()
		if err != nil {
			t.Fatalf("second connection after the first closed: %v", err)
		}
		if v != "x" {
			t.Fatalf("second connection read %q, want %q", v, "x")
		}
		return
	}

	select {
	case got := <-done:
		t.Fatalf(
			"second connection completed (%q, %v) while the first holds the database",
			got.v,
			got.err,
		)
	case <-time.After(200 * time.Millisecond):
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("second connection after the first closed: %v", got.err)
		}
		if got.v != "x" {
			t.Fatalf("second connection read %q, want %q", got.v, "x")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("second connection still blocked after the first closed")
	}
}
