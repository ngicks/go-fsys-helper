package overlay

import (
	"path"
	"path/filepath"
)

// MetadataStore stores metadata for [Layer].
//
// For example implementation, use [MetadataStoreSimpleText],
// which is just a text file of new line delimtied paths.
//
// If you want more efficient implementation for more many files,
// implement your own using more sophistcated data storage, e.g. SQLite.
type MetadataStore interface {
	// RecordWhiteout records named path as whited-out.
	// name is cleaned using [filepath.Clean] and [filepath.ToSlash].
	// RecordWhiteout rejects path that would be "." after cleaning.
	RecordWhiteout(name string) error
	// RemoveWhiteout removes name from record.
	RemoveWhiteout(name string) error
	// QueryWhiteout queries records and returns true if name or parent of name is whited-out.
	QueryWhiteout(name string) (has bool, err error)
}

var _ MetadataStore = (*subMetadataStore)(nil)

type subMetadataStore struct {
	base string
	org  MetadataStore
}

func (s *subMetadataStore) QueryWhiteout(name string) (has bool, err error) {
	name = filepath.Clean(name)
	return s.org.QueryWhiteout(path.Join(s.base, filepath.ToSlash(name)))
}

func (s *subMetadataStore) RecordWhiteout(name string) error {
	name = filepath.Clean(name)
	return s.org.RecordWhiteout(path.Join(s.base, filepath.ToSlash(name)))
}

func (s *subMetadataStore) RemoveWhiteout(name string) error {
	name = filepath.Clean(name)
	return s.org.RemoveWhiteout(path.Join(s.base, filepath.ToSlash(name)))
}

func SubMetadataStore(store MetadataStore, base string) MetadataStore {
	base = filepath.ToSlash(filepath.Clean(base))
	return &subMetadataStore{
		base: base,
		org:  store,
	}
}
