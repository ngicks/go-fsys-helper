package overlayfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ CopyUpPolicy = (*CopyUpPolicyStage)(nil)

// CopyUpPolicyStage publishes a copied-up regular file via a temp that lives in
// an explicit, VISIBLE work dir at the top root (workDir, default
// [DefaultWorkDir]), then renames it onto the full destination path within the
// single top Fs (atomic). All copy-up scratch is corralled in one known dir
// instead of beside content.
//
// The work dir is not hidden — it appears in the overlay's merged view like any
// other entry, mirroring Linux overlay's caller-designated workdir. On the
// FIRST CopyUp the policy lazily creates the work dir and self-sweeps leftover
// "*.copyup.tmp" files from a crashed run (the distinctive suffix makes the
// glob safe). The lazy-create + sweep runs at most once and is concurrency-safe
// (the policy may be shared across goroutines / sub-overlays).
//
// For directories and symlinks the policy behaves like
// [CopyUpPolicyDotTmp]: they are created directly at the destination (those ops
// are already atomic and need no temp).
type CopyUpPolicyStage struct {
	workDir string

	once    sync.Once
	prepErr error
}

// NewCopyUpPolicyStage returns a [CopyUpPolicyStage] whose temps live in
// workDir at the top root. An empty workDir selects [DefaultWorkDir].
func NewCopyUpPolicyStage(workDir string) *CopyUpPolicyStage {
	if workDir == "" {
		workDir = DefaultWorkDir
	}
	return &CopyUpPolicyStage{workDir: path.Clean(workDir)}
}

// prepare lazily creates the work dir and sweeps leftover *.copyup.tmp files
// from a crashed run. It runs at most once across all callers; the same result
// is returned on every subsequent call.
func (c *CopyUpPolicyStage) prepare(to vroot.Fs[vroot.File]) error {
	c.once.Do(func() {
		if err := to.MkdirAll(c.workDir, 0o777); err != nil {
			c.prepErr = fmt.Errorf("copy-up stage: mkdir work dir %q: %w", c.workDir, err)
			return
		}
		c.prepErr = c.sweep(to)
	})
	return c.prepErr
}

// sweep removes leftover copy-up temps (entries ending in ".copyup.tmp") from
// the work dir. Missing entries / removal of an entry that vanished
// concurrently are tolerated.
func (c *CopyUpPolicyStage) sweep(to vroot.Fs[vroot.File]) error {
	dirents, err := vroot.ReadDir(to, c.workDir)
	if err != nil {
		return fmt.Errorf("copy-up stage: read work dir %q: %w", c.workDir, err)
	}
	for _, de := range dirents {
		if !strings.HasSuffix(de.Name(), copyupTempSuffix) {
			continue
		}
		stale := path.Join(c.workDir, de.Name())
		if err := to.Remove(stale); err != nil && !isStaleRemovable(err) {
			return fmt.Errorf("copy-up stage: sweep %q: %w", stale, err)
		}
	}
	return nil
}

// isStaleRemovable reports whether a Remove error during the sweep can be
// ignored (the entry already vanished concurrently).
func isStaleRemovable(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// CopyUp implements [CopyUpPolicy]. The temp for a regular file is created in
// the work dir and renamed onto the full destination path within to.
func (c *CopyUpPolicyStage) CopyUp(from, to vroot.Fs[vroot.File], name string) error {
	if err := c.prepare(to); err != nil {
		return err
	}
	// nameMax is fixed at the default internally for the staged temp; the work
	// dir is a single component so the budget applies to the temp's own name.
	tempPattern := copyupTempPattern(name, defaultNameMax)
	return copyUp(from, to, name, c.workDir, tempPattern)
}
