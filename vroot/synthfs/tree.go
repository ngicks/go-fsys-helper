package synthfs

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// state owns the synthfs tree. Multiple [Root]s share the same state when one
// is derived from another via [Root.OpenRoot]; each Root only differs in which
// dir it treats as its confinement boundary.
type state struct {
	mu   sync.RWMutex
	opt  resolvedOption
	root *dir
}

// maxSymlinkResolution caps symlink walks to mirror Linux's ELOOP behavior.
const maxSymlinkResolution = 40

// normalize converts an external path (OS-form or slash-form) to the slash
// form synthfs uses internally, and rejects paths that escape the tree
// syntactically.
func normalize(name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." {
		return ".", nil
	}
	if !filepath.IsLocal(clean) {
		return "", vroot.ErrPathEscapes
	}
	return filepath.ToSlash(clean), nil
}

// resolveResult is what [state.resolve] returns: the resolved node plus the
// dir directly containing it. parent is nil only when the resolved node is the
// confinement-boundary dir itself (i.e. name resolved to ".").
type resolveResult struct {
	node   node
	parent *dir
	base   string
}

// resolve walks name starting from start, treating boundary as the outermost
// dir: ".." past boundary, or an absolute symlink ("/..."), is rejected with
// [vroot.ErrPathEscapes]. Intermediate symlinks are always followed; the
// trailing component is followed iff followLast is true.
//
// Caller must hold state.mu (read lock is sufficient — resolve never
// mutates the tree).
func (s *state) resolve(
	boundary, start *dir,
	name string,
	followLast bool,
	op string,
) (resolveResult, error) {
	slash, err := normalize(name)
	if err != nil {
		return resolveResult{}, fsutil.WrapPathErr(op, name, err)
	}
	if slash == "." {
		return resolveResult{node: start}, nil
	}

	cur := start
	rest := slash
	var symlinkHops int

	for rest != "" {
		var comp string
		comp, rest, _ = strings.Cut(rest, "/")
		switch comp {
		case "", ".":
			continue
		case "..":
			if cur == boundary {
				return resolveResult{}, fsutil.WrapPathErr(op, name, vroot.ErrPathEscapes)
			}
			if cur.parent == nil {
				// Defensive: a non-boundary dir without a parent shouldn't be
				// reachable, but treat it as an escape rather than panic.
				return resolveResult{}, fsutil.WrapPathErr(op, name, vroot.ErrPathEscapes)
			}
			cur = cur.parent
			continue
		}

		child := cur.lookup(comp)
		if child == nil {
			return resolveResult{}, fsutil.WrapPathErr(op, name, fs.ErrNotExist)
		}

		isLast := rest == ""

		if sym, ok := child.(*symlink); ok && (!isLast || followLast) {
			symlinkHops++
			if symlinkHops > maxSymlinkResolution {
				return resolveResult{}, fsutil.WrapPathErr(op, name, errdef.ELOOP)
			}
			target := sym.target
			// Absolute targets can't be resolved while staying confined.
			if strings.HasPrefix(target, "/") {
				return resolveResult{}, fsutil.WrapPathErr(op, name, vroot.ErrPathEscapes)
			}
			if rest == "" {
				rest = target
			} else {
				rest = target + "/" + rest
			}
			continue
		}

		if isLast {
			return resolveResult{node: child, parent: cur, base: comp}, nil
		}

		// More path to traverse: must be a dir.
		nextDir, ok := child.(*dir)
		if !ok {
			return resolveResult{}, fsutil.WrapPathErr(op, name, syscall.ENOTDIR)
		}
		cur = nextDir
	}
	// rest became empty after stripping "."/".."/repeated "/"; we landed on cur.
	return resolveResult{node: cur}, nil
}

// resolveParent splits name into (parent, base) and resolves the parent dir.
// The trailing base component is not looked up. Used by operations that
// create or remove a leaf at a known parent.
func (s *state) resolveParent(
	boundary, start *dir,
	name, op string,
) (parent *dir, base string, err error) {
	slash, err := normalize(name)
	if err != nil {
		return nil, "", fsutil.WrapPathErr(op, name, err)
	}
	if slash == "." {
		// Creating/removing "." is never valid.
		return nil, "", fsutil.WrapPathErr(op, name, fs.ErrInvalid)
	}
	dirPart, base := path.Split(slash)
	dirPart = strings.TrimSuffix(dirPart, "/")
	if dirPart == "" {
		dirPart = "."
	}
	res, err := s.resolve(boundary, start, dirPart, true, op)
	if err != nil {
		return nil, "", err
	}
	pd, ok := res.node.(*dir)
	if !ok {
		return nil, "", fsutil.WrapPathErr(op, name, syscall.ENOTDIR)
	}
	return pd, base, nil
}

// fileForView returns a fresh stat reflecting v's current size; mode and
// modTime stay with the node, only size is authoritative on the view.
func statFromFileView(m *metadata, v FileView) (stat, error) {
	info, err := v.Stat()
	if err != nil {
		return stat{}, err
	}
	s := m.toStat()
	s.size = info.Size()
	return s, nil
}

// chmodApply replaces the perm bits of m.mode through the configured
// MaskChmodMode while preserving type bits (ModeDir/ModeSymlink/…).
// Caller must hold state.mu (write lock).
func chmodApply(opt *resolvedOption, m *metadata, mode fs.FileMode) {
	// Preserve type bits; recompute everything else through MaskChmodMode so
	// platform emulation (e.g. fsutil.MaskChmodModeWindows) gets the chance
	// to widen or narrow as it sees fit.
	const extraBits = fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky |
		fs.ModeAppend | fs.ModeExclusive | fs.ModeTemporary
	typeBits := m.mode & ^fs.ModePerm & ^extraBits
	m.mode = typeBits | opt.maskChmodMode(mode)
}

// Compile-time assertion that *Root will satisfy the generic root constraint
// once root.go declares it. Kept here to anchor the import set when the
// package would otherwise have nothing referencing vroot.
var _ = vroot.ErrPathEscapes
