package sqlvfs

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// DB is a sqlite database opened inside a [vroot.Fs]. It owns the [FS] that
// serves it; Close closes the database and unregisters the VFS, but leaves the
// fsys itself to its owner.
type DB struct {
	*sql.DB
	fs *FS
}

// Open opens, creating it if absent, the sqlite database at name inside fsys.
//
// The connection runs locking_mode=EXCLUSIVE over a rollback journal: the
// overlay is the database's only user, and no journal mode needing shared
// memory can work over a VFS that has none. Extra pragmas are applied after
// those two, in the order given.
func Open(fsys vroot.Fs[vroot.File], name string, pragmas ...string) (*DB, error) {
	return open(fsys, name, url.Values{
		"_pragma": append([]string{"locking_mode(exclusive)", "journal_mode(delete)"}, pragmas...),
	})
}

// OpenReadOnly opens the sqlite database at name inside fsys for reading. It
// must already exist, and every write through the returned DB fails.
//
// immutable goes further than mode=ro alone: sqlite stops looking for a hot
// rollback journal to replay — a replay no read-only backend could carry out
// anyway — and stops locking the file, so several readers can share one
// database. In exchange a database some crashed writer left hot reads as it
// stands, un-rolled-back; opening one read-only says nobody is writing it.
func OpenReadOnly(fsys vroot.Fs[vroot.File], name string) (*DB, error) {
	return open(fsys, name, url.Values{"mode": {"ro"}, "immutable": {"1"}})
}

func open(fsys vroot.Fs[vroot.File], name string, params url.Values) (*DB, error) {
	f := Register(fsys)
	db, err := driver.Open(f.DSN(name, params))
	if err == nil {
		// An exclusive-locking connection cannot share the database with a second
		// one, so the pool must never open one; a read-only connection could, but
		// the store drives it one call at a time either way.
		db.SetMaxOpenConns(1)
		// driver.Open connects lazily; Ping is what creates the file on a writable
		// open, and what reports a broken fsys — or a database a read-only open
		// cannot find — here instead of at the store's first query.
		if err = db.Ping(); err != nil {
			err = errors.Join(err, db.Close())
		}
	}
	if err != nil {
		f.Unregister()
		return nil, fmt.Errorf("sqlvfs: open %q in %s: %w", name, fsys.Name(), err)
	}
	return &DB{DB: db, fs: f}, nil
}

// Close closes the database and unregisters its VFS.
func (db *DB) Close() error {
	err := db.DB.Close()
	db.fs.Unregister()
	return err
}
