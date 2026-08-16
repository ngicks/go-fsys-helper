package overlayfs

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/internal/sqlvfs"
)

var _ MetadataStore = (*MetadataStoreSQLite)(nil)

// schemaVersion is the version schema_info carries. A database written by any
// other version is rejected rather than migrated: this is v1, there is nothing
// to migrate from yet.
const schemaVersion = "1"

// The node table is keyed by the full path and declared WITHOUT ROWID, which
// makes the table itself a clustered B+Tree over paths: a subtree is one
// contiguous range scan. parent keeps the adjacency view for direct-children
// queries, which would otherwise have to scan every descendant.
const (
	ddlNode = `
CREATE TABLE IF NOT EXISTS node (
    path     TEXT NOT NULL PRIMARY KEY,   -- full clean slash path; '' = root
    parent   TEXT NOT NULL,               -- dirname(path); '' for top-level
    whiteout INTEGER NOT NULL DEFAULT 0,  -- 0|1
    opaque   INTEGER NOT NULL DEFAULT 0   -- 0|1
) STRICT, WITHOUT ROWID`
	ddlNodeParent = `CREATE INDEX IF NOT EXISTS node_parent ON node (parent)`
	ddlSchemaInfo = `
CREATE TABLE IF NOT EXISTS schema_info (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT`
	selectSchemaVersion = `SELECT value FROM schema_info WHERE key = 'version'`
)

// MetadataStoreSQLite is the [MetadataStore] the overlay uses by default: a
// sqlite database living inside the layer's own [vroot.Fs], reached through a
// VFS built on that fsys rather than on OS paths, so a fully virtual layer
// keeps durable masking state too.
//
// Each setter commits its own transaction before returning. A store opened for
// writing holds the database in locking_mode=EXCLUSIVE, so it must be closed
// before the same database can be opened again; a read-only one takes no lock
// at all and any number of them can read one database at once.
type MetadataStoreSQLite struct {
	db *sqlvfs.DB
}

// NewMetadataStoreSQLite opens, creating it if absent, the metadata store at
// name inside fsys. An existing database must carry the schema version this
// package writes; anything else is an error.
func NewMetadataStoreSQLite[F vroot.File](
	fsys vroot.Fs[F],
	name string,
) (*MetadataStoreSQLite, error) {
	db, err := sqlvfs.Open(vroot.Widen(fsys), name)
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		// Close is also what gives the VFS registration back.
		return nil, errors.Join(err, db.Close())
	}
	return &MetadataStoreSQLite{db: db}, nil
}

// newMetadataStoreSQLiteReadOnly opens the store at name inside fsys without
// ever writing to it, for a layer whose backend refuses writes. The schema is
// verified but never created: a database that cannot be written cannot be
// brought into shape either, so an absent or foreign one is an error here where
// [NewMetadataStoreSQLite] would have created or rejected it.
func newMetadataStoreSQLiteReadOnly(
	fsys vroot.Fs[vroot.File],
	name string,
) (*MetadataStoreSQLite, error) {
	db, err := sqlvfs.OpenReadOnly(fsys, name)
	if err != nil {
		return nil, err
	}
	if err := checkSchemaVersion(db.QueryRow(selectSchemaVersion)); err != nil {
		return nil, errors.Join(fmt.Errorf("overlayfs: metadata schema: %w", err), db.Close())
	}
	return &MetadataStoreSQLite{db: db}, nil
}

// checkSchemaVersion fails unless row carries the version this package writes.
func checkSchemaVersion(row *sql.Row) error {
	var version string
	if err := row.Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("version is %q, want %q", version, schemaVersion)
	}
	return nil
}

func initSchema(db *sqlvfs.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("overlayfs: metadata schema: %w", err)
	}
	err = func() error {
		for _, stmt := range []string{ddlNode, ddlNodeParent, ddlSchemaInfo} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		// The IF NOT EXISTS above and the OR IGNORE here leave a database written
		// by another version untouched, so the check below still reads the version
		// that database was created with.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO schema_info (key, value) VALUES ('version', ?)`,
			schemaVersion,
		); err != nil {
			return err
		}
		return checkSchemaVersion(tx.QueryRow(selectSchemaVersion))
	}()
	if err != nil {
		return errors.Join(fmt.Errorf("overlayfs: metadata schema: %w", err), tx.Rollback())
	}
	return tx.Commit()
}

// Load implements [MetadataStore].
func (s *MetadataStoreSQLite) Load() (whiteouts []string, opaques []string, err error) {
	rows, err := s.db.Query(`SELECT path, whiteout, opaque FROM node`)
	if err != nil {
		return nil, nil, fmt.Errorf("overlayfs: metadata load: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			name     string
			whiteout bool
			opaque   bool
		)
		if err := rows.Scan(&name, &whiteout, &opaque); err != nil {
			return nil, nil, fmt.Errorf("overlayfs: metadata load: %w", err)
		}
		if whiteout {
			whiteouts = append(whiteouts, name)
		}
		if opaque {
			opaques = append(opaques, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("overlayfs: metadata load: %w", err)
	}
	return whiteouts, opaques, nil
}

// SetWhiteout implements [MetadataStore].
func (s *MetadataStoreSQLite) SetWhiteout(name string) error {
	name = cleanName(name)
	return s.exec("set whiteout", name, func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO node (path, parent, whiteout, opaque) VALUES (?, ?, 1, 0)
             ON CONFLICT (path) DO UPDATE SET whiteout = 1, opaque = 0`,
			name, parentName(name),
		); err != nil {
			return err
		}
		return deleteSubtree(tx, name)
	})
}

// ClearWhiteout implements [MetadataStore].
func (s *MetadataStoreSQLite) ClearWhiteout(name string) error {
	name = cleanName(name)
	return s.exec("clear whiteout", name, func(tx *sql.Tx) error {
		return clearMark(tx, `whiteout`, name)
	})
}

// SetOpaque implements [MetadataStore].
func (s *MetadataStoreSQLite) SetOpaque(dir string) error {
	dir = cleanName(dir)
	return s.exec("set opaque", dir, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO node (path, parent, whiteout, opaque) VALUES (?, ?, 0, 1)
             ON CONFLICT (path) DO UPDATE SET opaque = 1`,
			dir, parentName(dir),
		)
		return err
	})
}

// ClearOpaque implements [MetadataStore].
func (s *MetadataStoreSQLite) ClearOpaque(dir string) error {
	dir = cleanName(dir)
	return s.exec("clear opaque", dir, func(tx *sql.Tx) error {
		return clearMark(tx, `opaque`, dir)
	})
}

// Close implements [MetadataStore].
func (s *MetadataStoreSQLite) Close() error {
	return s.db.Close()
}

// exec runs fn in a transaction that has committed by the time it returns —
// the write-through the setters promise.
func (s *MetadataStoreSQLite) exec(op string, name string, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err == nil {
		if err = fn(tx); err != nil {
			err = errors.Join(err, tx.Rollback())
		} else {
			err = tx.Commit()
		}
	}
	if err != nil {
		return fmt.Errorf("overlayfs: metadata %s %q: %w", op, name, err)
	}
	return nil
}

// clearMark drops one mark from name and, with the other mark already gone,
// the row itself: a row exists only while it carries state.
func clearMark(tx *sql.Tx, column string, name string) error {
	// column is one of two literals chosen here, never caller input; sqlite has
	// no parameter form for an identifier.
	if _, err := tx.Exec(`UPDATE node SET `+column+` = 0 WHERE path = ?`, name); err != nil {
		return err
	}
	_, err := tx.Exec(
		`DELETE FROM node WHERE path = ? AND whiteout = 0 AND opaque = 0`,
		name,
	)
	return err
}

// deleteSubtree drops every row strictly under name. The range works because
// '0' is the byte after '/' and the default BINARY collation compares
// bytewise, so it covers exactly the names prefixed with name+"/" — but it
// cannot express the root, whose subtree is the whole table: no stored path
// starts with '/'.
func deleteSubtree(tx *sql.Tx, name string) error {
	if name == "" {
		_, err := tx.Exec(`DELETE FROM node WHERE path <> ''`)
		return err
	}
	_, err := tx.Exec(
		`DELETE FROM node WHERE path >= ? || '/' AND path < ? || '0'`,
		name, name,
	)
	return err
}
