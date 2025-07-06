// Package acceptancetest defines tests for interface vroot.Rooted and vroot.Unrooted.
// Tests are loose constraint for the interfaces:
// Some behaviors, mostly corner cases, may vary between implementations and acceptancetest allows it.
// Callers may also use fstest.TestFS for stricter reader side tests
// by converting vroot.Fs into fs.FS with vroot.ToIoFsRooted or vroot.ToIoFsUnrooted.
package acceptancetest

type Option struct {
	FileRemovalPolicy FileRemovalPolicy
}

type FileRemovalPolicy string

const (
	// Can remove opened files on unix/plan9,
	// not on windows.
	FileRemovalPolicyPlatformSpecific    FileRemovalPolicy = "platform-specific"
	FileRemovalPolicyCanRemoveOpened     FileRemovalPolicy = "can-remove-opened"
	FileRemovalPolicyCanNotRemoveOopened FileRemovalPolicy = "can-not-remove-opened"
)
