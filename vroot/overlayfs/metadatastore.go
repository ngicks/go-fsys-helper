package overlayfs

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// ErrInvalidPath is returned by [MetadataStore] implementations when a name
// fails validation (empty, ".", or containing ".."). Names are normalized to
// clean forward-slash paths and must satisfy [fs.ValidPath] while never being
// the root ".".
var ErrInvalidPath = errors.New("invalid metadata path")

// MetadataStore is the durable journal that seeds and records the overlay's
// whiteout and opaque deviation-state.
//
// The overlay's in-memory virtual-node table is the authoritative truth; the
// store carries NO query methods. It is loaded once at construction (Load) to
// seed the nodes, then durably records single changes (write-behind, off the
// hot path) as they happen.
//
// All names are clean forward-slash paths (see [fs.ValidPath]); the root "."
// and ".."-containing paths are rejected with [ErrInvalidPath].
type MetadataStore interface {
	// Load returns all persisted whiteout and opaque paths so the ovl-nodes can
	// be seeded at construction. Called once. Paths are clean slash paths.
	Load() (whiteouts []string, opaques []string, err error)
	// SetWhiteout records name as a subtree-masking whiteout: name and
	// everything that would live under it in the lowers is hidden.
	SetWhiteout(name string) error
	// ClearWhiteout removes a previously recorded whiteout.
	ClearWhiteout(name string) error
	// SetOpaque records dir as an exact-dir opaque: lower children of dir are
	// hidden while top children of dir remain visible.
	SetOpaque(dir string) error
	// ClearOpaque removes a previously recorded opaque marker.
	ClearOpaque(dir string) error
	// Flush forces pending writes durable. It is a no-op for synchronous stores.
	Flush() error
}

// validateName normalizes name to a clean forward-slash path and validates it
// the way the old store did: reject empty, ".", and any ".." component.
func validateName(name string) (string, error) {
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == "" {
		return "", ErrInvalidPath
	}
	// fs.ValidPath rejects leading "/", ".." components, and ".".
	if !fs.ValidPath(cleaned) {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

var _ MetadataStore = (*subMetadataStore)(nil)

// subMetadataStore prefixes every name with base before delegating, and strips
// base from Load results. See [SubMetadataStore].
type subMetadataStore struct {
	base string
	org  MetadataStore
}

// SubMetadataStore returns a [MetadataStore] that maps a sub-overlay's
// keyspace into the parent's: every name is prefixed with base (via
// path.Join) before delegating to s, and Load returns only the entries under
// base with the base prefix removed. base is a clean slash path.
func SubMetadataStore(s MetadataStore, base string) MetadataStore {
	base = path.Clean(base)
	if base == "." {
		base = ""
	}
	return &subMetadataStore{
		base: base,
		org:  s,
	}
}

func (s *subMetadataStore) join(name string) (string, error) {
	cleaned, err := validateName(name)
	if err != nil {
		return "", err
	}
	if s.base == "" {
		return cleaned, nil
	}
	return path.Join(s.base, cleaned), nil
}

// strip removes the base prefix from a full path, reporting whether the path
// is under base.
func (s *subMetadataStore) strip(full string) (string, bool) {
	if s.base == "" {
		return full, true
	}
	if full == s.base {
		// Exactly base: nothing remains under it for this view.
		return "", false
	}
	prefix := s.base + "/"
	if !strings.HasPrefix(full, prefix) {
		return "", false
	}
	rest := full[len(prefix):]
	if rest == "" {
		return "", false
	}
	return rest, true
}

func (s *subMetadataStore) Load() (whiteouts []string, opaques []string, err error) {
	w, o, err := s.org.Load()
	if err != nil {
		return nil, nil, err
	}
	whiteouts = make([]string, 0, len(w))
	for _, p := range w {
		if stripped, ok := s.strip(p); ok {
			whiteouts = append(whiteouts, stripped)
		}
	}
	opaques = make([]string, 0, len(o))
	for _, p := range o {
		if stripped, ok := s.strip(p); ok {
			opaques = append(opaques, stripped)
		}
	}
	return whiteouts, opaques, nil
}

func (s *subMetadataStore) SetWhiteout(name string) error {
	full, err := s.join(name)
	if err != nil {
		return err
	}
	return s.org.SetWhiteout(full)
}

func (s *subMetadataStore) ClearWhiteout(name string) error {
	full, err := s.join(name)
	if err != nil {
		return err
	}
	return s.org.ClearWhiteout(full)
}

func (s *subMetadataStore) SetOpaque(dir string) error {
	full, err := s.join(dir)
	if err != nil {
		return err
	}
	return s.org.SetOpaque(full)
}

func (s *subMetadataStore) ClearOpaque(dir string) error {
	full, err := s.join(dir)
	if err != nil {
		return err
	}
	return s.org.ClearOpaque(full)
}

func (s *subMetadataStore) Flush() error {
	return s.org.Flush()
}
