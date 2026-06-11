package synthfs

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot/internal/paths"
)

// scaffoldDirMode is the mode used for parent directories synthfs creates on
// the way to a mount point that the caller did not pre-create. The plan
// (§3 "Resolved-but-confirm") locks this to `0o755 &^ Umask`.
const scaffoldDirMode fs.FileMode = 0o755

// AddFile binds view as a regular file at name. Intermediate directories that
// don't exist yet are scaffolded with mode 0o755 (masked by Umask). The
// node's mode is taken verbatim from view.Stat() — Umask is not applied.
//
// cb decides what to do when an entry already exists at name. nil ⇒
// [MergeOverwrite] (overwrites an existing leaf).
//
// On error the tree is left in whatever state the partial ingest produced;
// callers that need atomic ingest should build into a fresh [Root] and swap
// at the end.
func (r *Root) AddFile(name string, view FileView, cb AddFunc) error {
	if cb == nil {
		cb = MergeOverwrite
	}
	info, err := view.Stat()
	if err != nil {
		return fsutil.WrapPathErr("AddFile", name, err)
	}
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	parent, base, err := r.ensureParents(name, "AddFile")
	if err != nil {
		return err
	}
	slashPath := slashJoin(parentSlashOf(name), base)
	incoming := &addEntry{p: slashPath, info: info}
	existingNode := parent.lookup(base)
	var existing AddEntry
	if existingNode != nil {
		existing = nodeAsAddEntry(slashPath, existingNode)
	}
	decision, cbErr := cb(incoming, existing)
	if cbErr != nil {
		return cbErr
	}
	switch decision {
	case AddDecisionSkip:
		return nil
	case AddDecisionKeep:
		if existingNode == nil {
			return nil
		}
		// File-on-file Keep: drop incoming. File-on-dir Keep would have been
		// rejected upstream by sane resolvers; treat it as a no-op here.
		return nil
	case AddDecisionOverride:
		if existingNode != nil {
			parent.removeEntry(base)
			if f, ok := existingNode.(*file); ok {
				_ = f.view.Close()
			}
		}
		parent.addEntry(base, &file{
			metadata: metadata{
				name:    base,
				mode:    info.Mode() & (fs.ModePerm | (fs.ModeType &^ fs.ModeSymlink &^ fs.ModeDir)),
				modTime: info.ModTime(),
				size:    info.Size(),
			},
			view: view,
		})
		return nil
	}
	return fsutil.WrapPathErr("AddFile", name, errors.New("synthfs: unknown AddDecision"))
}

// AddFS walks fsys depth-first and binds a read-only view for each regular
// file, recreating directories along the way. If fsys also implements
// [fs.ReadLinkFS], symlinks are reproduced as synth-tree symlinks.
//
// Intermediate parent directories of name that don't yet exist are scaffolded
// (mode 0o755 &^ Umask). For each source entry, cb is consulted (nil ⇒
// [MergeOverwrite]). The walk is depth-first; a directory's children are
// visited only when cb returns [AddDecisionKeep] (merge) or
// [AddDecisionOverride] for that directory.
func (r *Root) AddFS(name string, fsys fs.FS, cb AddFunc) error {
	if cb == nil {
		cb = MergeOverwrite
	}
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	rl, _ := fsys.(fs.ReadLinkFS)

	slash, err := normalize(name)
	if err != nil {
		return fsutil.WrapPathErr("AddFS", name, err)
	}
	// AddFS(".") means "merge fsys's contents into r's boundary dir." The
	// boundary itself is not a candidate for the resolver — we go straight
	// to descending its children.
	if slash == "." {
		return r.descend(r.boundary, ".", fsys, rl, ".", cb)
	}

	parent, base, err := r.ensureParents(name, "AddFS")
	if err != nil {
		return err
	}
	mountSlash := slashJoin(parentSlashOf(name), base)
	return r.addFSWalk(parent, base, mountSlash, fsys, rl, ".", cb)
}

// addFSWalk implements the recursive walk inside the AddFS write-lock region.
// dst is the dir under which the entry will land (parent of base in the
// synth tree); base is the basename; mountSlash is the slash path of the
// entry; src is the path in fsys.
func (r *Root) addFSWalk(
	dst *dir,
	base, mountSlash string,
	fsys fs.FS,
	rl fs.ReadLinkFS,
	src string,
	cb AddFunc,
) error {
	var (
		info fs.FileInfo
		err  error
	)
	if rl != nil {
		info, err = rl.Lstat(src)
	} else {
		info, err = fs.Stat(fsys, src)
	}
	if err != nil {
		return err
	}

	incoming := &addEntry{p: mountSlash, info: info}
	existingNode := dst.lookup(base)
	var existing AddEntry
	if existingNode != nil {
		existing = nodeAsAddEntry(mountSlash, existingNode)
	}
	decision, cbErr := cb(incoming, existing)
	if cbErr != nil {
		return cbErr
	}

	switch {
	case info.IsDir():
		switch decision {
		case AddDecisionSkip:
			return nil
		case AddDecisionKeep:
			// Reuse or create + descend.
			d := r.materializeDir(dst, base, info, existingNode, false)
			return r.descend(d, mountSlash, fsys, rl, src, cb)
		case AddDecisionOverride:
			d := r.materializeDir(dst, base, info, existingNode, true)
			return r.descend(d, mountSlash, fsys, rl, src, cb)
		}
	case info.Mode()&fs.ModeSymlink != 0:
		if decision == AddDecisionSkip {
			return nil
		}
		if rl == nil {
			return fsutil.WrapPathErr(
				"AddFS",
				mountSlash,
				errors.New("synthfs: source fs does not implement fs.ReadLinkFS"),
			)
		}
		target, err := rl.ReadLink(src)
		if err != nil {
			return err
		}
		if decision == AddDecisionOverride && existingNode != nil {
			dst.removeEntry(base)
			if f, ok := existingNode.(*file); ok {
				_ = f.view.Close()
			}
		}
		if decision == AddDecisionKeep && existingNode != nil {
			return nil
		}
		dst.addEntry(base, &symlink{
			metadata: metadata{
				name:    base,
				mode:    fs.ModeSymlink | (info.Mode() & fs.ModePerm),
				modTime: info.ModTime(),
			},
			target: filepath.ToSlash(target),
		})
		return nil
	default: // regular file
		if decision == AddDecisionSkip {
			return nil
		}
		view, err := NewFsView(fsys, src)
		if err != nil {
			return err
		}
		if decision == AddDecisionKeep && existingNode != nil {
			_ = view.Close()
			return nil
		}
		if existingNode != nil {
			dst.removeEntry(base)
			if f, ok := existingNode.(*file); ok {
				_ = f.view.Close()
			}
		}
		dst.addEntry(base, &file{
			metadata: metadata{
				name:    base,
				mode:    info.Mode() & fs.ModePerm,
				modTime: info.ModTime(),
				size:    info.Size(),
			},
			view: view,
		})
	}
	return nil
}

// materializeDir ensures a *dir exists at base under dst, reusing the
// existing entry when possible. If override is true, an existing dir is
// replaced (its contents are pruned first). Returns the resulting dir.
// Caller holds state.mu (write).
func (r *Root) materializeDir(
	dst *dir,
	base string,
	info fs.FileInfo,
	existing node,
	override bool,
) *dir {
	if d, ok := existing.(*dir); ok && !override {
		return d
	}
	if existing != nil {
		dst.removeEntry(base)
		if override {
			r.removeSubtree(existing)
		}
	}
	d := newDir(base, info.Mode()&fs.ModePerm, info.ModTime(), dst)
	dst.addEntry(base, d)
	return d
}

// descend visits the children of src inside fsys and dispatches each through
// addFSWalk.
func (r *Root) descend(
	d *dir,
	parentSlash string,
	fsys fs.FS,
	rl fs.ReadLinkFS,
	src string,
	cb AddFunc,
) error {
	entries, err := fs.ReadDir(fsys, src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childSrc := path.Join(src, e.Name())
		childSlash := slashJoin(parentSlash, e.Name())
		if err := r.addFSWalk(d, e.Name(), childSlash, fsys, rl, childSrc, cb); err != nil {
			return err
		}
	}
	return nil
}

// ensureParents resolves name into (parent, base) and creates missing
// intermediate dirs along the way with mode `scaffoldDirMode &^ Umask`.
// Caller holds state.mu (write).
func (r *Root) ensureParents(name, op string) (*dir, string, error) {
	slash, err := normalize(name)
	if err != nil {
		return nil, "", fsutil.WrapPathErr(op, name, err)
	}
	if slash == "." {
		return nil, "", fsutil.WrapPathErr(op, name, fs.ErrInvalid)
	}
	dirPart, base := path.Split(slash)
	dirPart = strings.TrimSuffix(dirPart, "/")
	if dirPart == "" {
		dirPart = "."
	}
	parent := r.boundary
	if dirPart != "." {
		for p := range paths.PathFromHead(filepath.FromSlash(dirPart)) {
			pSlash := filepath.ToSlash(p)
			res, err := r.st.resolve(r.boundary, r.boundary, pSlash, true, op)
			if err == nil {
				d, ok := res.node.(*dir)
				if !ok {
					return nil, "", fsutil.WrapPathErr(op, name, syscall.ENOTDIR)
				}
				parent = d
				continue
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, "", err
			}
			// Create here.
			pParent, pBase, perr := r.st.resolveParent(r.boundary, r.boundary, pSlash, op)
			if perr != nil {
				return nil, "", perr
			}
			d := newDir(pBase, scaffoldDirMode&^r.st.opt.umask, r.st.opt.clock.Now(), pParent)
			pParent.addEntry(pBase, d)
			parent = d
		}
	}
	return parent, base, nil
}

// nodeAsAddEntry wraps an existing tree node so resolvers see a uniform
// [AddEntry] view of it. Caller holds state.mu (read).
func nodeAsAddEntry(slashPath string, n node) AddEntry {
	switch nd := n.(type) {
	case *file:
		info, err := statFromFileView(&nd.metadata, nd.view)
		if err != nil {
			return &addEntry{p: slashPath, info: nd.toStat()}
		}
		return &addEntry{p: slashPath, info: info}
	default:
		return &addEntry{p: slashPath, info: n.meta().toStat()}
	}
}

// parentSlashOf returns the slash-form parent of name (or "." if name has no
// parent). Used to compute the slash path for AddEntry.
func parentSlashOf(name string) string {
	slash, _ := normalize(name)
	if slash == "." {
		return "."
	}
	dir, _ := path.Split(slash)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return "."
	}
	return dir
}

// slashJoin joins parent and base, collapsing "." to just base.
func slashJoin(parent, base string) string {
	if parent == "." || parent == "" {
		return base
	}
	return parent + "/" + base
}
