package overlayfs

import (
	"path"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ CopyUpPolicy = (*CopyUpPolicyDotTmp)(nil)

// CopyUpPolicyDotTmp publishes a copied-up regular file via a temp that lives
// BESIDE the destination (in the destination's own directory), then renames it
// onto the destination within the single top Fs (atomic). There is no standing
// work dir: the temp is named "<trunc-base>.<10 random>.copyup.tmp" and is
// renamed away immediately, so a completed listing is clean. On error the temp
// is removed; there is no directory-wide sweep.
//
// Directories and symlinks are created directly at the destination (those ops
// are already atomic and need no temp).
type CopyUpPolicyDotTmp struct {
	// nameMax is the per-component byte budget (NAME_MAX) the temp name must fit
	// within; the base is rune-boundary-truncated to nameMax-22 so
	// "<trunc-base>.<rand>.copyup.tmp" stays within nameMax.
	nameMax int
}

// NewCopyUpPolicyDotTmp returns a [CopyUpPolicyDotTmp]. nameMax bounds the
// temp file name's component length; 0 selects the default of 255.
func NewCopyUpPolicyDotTmp(nameMax int) *CopyUpPolicyDotTmp {
	if nameMax <= 0 {
		nameMax = defaultNameMax
	}
	return &CopyUpPolicyDotTmp{nameMax: nameMax}
}

// CopyUp implements [CopyUpPolicy]. The temp for a regular file is created in
// path.Dir(name) (beside the destination) so the publishing Rename is a single
// rename within to.
func (c *CopyUpPolicyDotTmp) CopyUp(from, to vroot.Fs[vroot.File], name string) error {
	tempDir := path.Dir(name)
	tempPattern := copyupTempPattern(name, c.nameMax)
	return copyUp(from, to, name, tempDir, tempPattern)
}
