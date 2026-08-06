package overlayfs

import (
	"slices"
	"sync"
)

var _ MetadataStore = (*MetadataStoreMem)(nil)

// MetadataStoreMem is an ephemeral [MetadataStore] with no durable
// persistence. It keeps two in-memory sets (whiteouts, opaques) guarded by a
// mutex. Load returns the current sets, so within a single process lifetime it
// round-trips. Flush is a no-op. It is safe for concurrent use.
type MetadataStoreMem struct {
	mu        sync.RWMutex
	whiteouts map[string]struct{}
	opaques   map[string]struct{}
}

// NewMetadataStoreMem returns an empty ephemeral metadata store.
func NewMetadataStoreMem() *MetadataStoreMem {
	return &MetadataStoreMem{
		whiteouts: make(map[string]struct{}),
		opaques:   make(map[string]struct{}),
	}
}

func (m *MetadataStoreMem) Load() (whiteouts []string, opaques []string, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	whiteouts = make([]string, 0, len(m.whiteouts))
	for p := range m.whiteouts {
		whiteouts = append(whiteouts, p)
	}
	opaques = make([]string, 0, len(m.opaques))
	for p := range m.opaques {
		opaques = append(opaques, p)
	}
	slices.Sort(whiteouts)
	slices.Sort(opaques)
	return whiteouts, opaques, nil
}

func (m *MetadataStoreMem) SetWhiteout(name string) error {
	cleaned, err := validateName(name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.whiteouts[cleaned] = struct{}{}
	return nil
}

func (m *MetadataStoreMem) ClearWhiteout(name string) error {
	cleaned, err := validateName(name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.whiteouts, cleaned)
	return nil
}

func (m *MetadataStoreMem) SetOpaque(dir string) error {
	cleaned, err := validateName(dir)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opaques[cleaned] = struct{}{}
	return nil
}

func (m *MetadataStoreMem) ClearOpaque(dir string) error {
	cleaned, err := validateName(dir)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.opaques, cleaned)
	return nil
}

// Flush is a no-op; MetadataStoreMem has no durable backing.
func (m *MetadataStoreMem) Flush() error {
	return nil
}
