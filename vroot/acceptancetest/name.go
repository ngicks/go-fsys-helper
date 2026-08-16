package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestName exercises [vroot.Fs.Name].
//
// Implementations expose a non-empty name. The exact value is implementation defined
// (osfs returns the absolute root path; in-memory file systems return a synthetic name).
func TestName[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	if got := fsys.Name(); got == "" {
		t.Errorf("Name() returned empty string")
	}
}
