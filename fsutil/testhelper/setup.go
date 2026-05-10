package testhelper

import (
	"cmp"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (c *C[T, F, Fs]) Setup(setups ...SetupProc[F, Fs]) {
	c.t.Helper()
	c.record("Setup(%d procs)", len(setups))
	ordered := slices.Clone(setups)
	slices.SortStableFunc(ordered, compareSetupProc[F, Fs])
	for _, setup := range ordered {
		c.record("SetupProc(%s)", describeSetupProc(setup))
		if err := setup.Setup(c.fsys); err != nil {
			c.ReportFailf("Setup: %v", err)
		}
	}
}

type SetupProc[F File, Fs Fsys[F]] interface {
	Path() string
	Order() int
	Setup(fsys Fs) error
}

func ParseSetupProcLine[F File, Fs Fsys[F]](line string) (SetupProc[F, Fs], error) {
	switch {
	case strings.Contains(line, " -> "):
		path, targetPath, _ := strings.Cut(line, " -> ")
		if path == "" || targetPath == "" {
			return nil, fmt.Errorf("invalid symlink setup line %q", line)
		}
		return &CreateSymlink[F, Fs]{
			Name:       filepath.FromSlash(path),
			TargetPath: filepath.FromSlash(targetPath),
		}, nil
	case strings.HasSuffix(line, "/"):
		path := strings.TrimSuffix(line, "/")
		if path == "" {
			return nil, fmt.Errorf("invalid directory setup line %q", line)
		}
		return &CreateDir[F, Fs]{Name: filepath.FromSlash(path)}, nil
	case strings.Contains(line, ": "):
		path, content, _ := strings.Cut(line, ": ")
		if path == "" {
			return nil, fmt.Errorf("invalid file setup line %q", line)
		}
		if strings.HasPrefix(content, `"`) || strings.HasPrefix(content, "`") {
			unquoted, err := strconv.Unquote(content)
			if err != nil {
				return nil, fmt.Errorf("invalid file setup line %q: %w", line, err)
			}
			content = unquoted
		} else if strings.Contains(content, " ") {
			return nil, fmt.Errorf("invalid file setup line %q: unquoted content must not contain spaces", line)
		}
		return &CreateFile[F, Fs]{
			Name:    filepath.FromSlash(path),
			Content: []byte(content),
		}, nil
	default:
		return nil, fmt.Errorf("unknown setup line %q", line)
	}
}

func compareSetupProc[F File, Fs Fsys[F]](l, r SetupProc[F, Fs]) int {
	return cmp.Or(
		cmp.Compare(l.Order(), r.Order()),
		cmp.Compare(pathDepth(l.Path()), pathDepth(r.Path())),
		cmp.Compare(l.Path(), r.Path()),
	)
}

func describeSetupProc[F File, Fs Fsys[F]](setup SetupProc[F, Fs]) string {
	return fmt.Sprintf("%T %q", setup, setup.Path())
}

func pathDepth(path string) int {
	path = filepath.Clean(path)
	if path == "." {
		return 0
	}
	depth := 1
	for _, r := range path {
		if r == filepath.Separator {
			depth++
		}
	}
	return depth
}

func mkdirParent[F File, Fs Fsys[F]](fsys Fs, path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	if err := fsys.MkdirAll(dir, fs.ModePerm); err != nil {
		return fmt.Errorf("MkdirAll %q: %w", dir, err)
	}
	return nil
}

type CreateDir[F File, Fs Fsys[F]] struct {
	Name  string
	Mode  fs.FileMode
	Mtime time.Time
}

func (p *CreateDir[F, Fs]) Path() string {
	return p.Name
}

func (p *CreateDir[F, Fs]) Order() int {
	return 0
}

func (p *CreateDir[F, Fs]) Setup(fsys Fs) error {
	if err := fsys.MkdirAll(p.Name, fs.ModePerm); err != nil {
		return fmt.Errorf("CreateDir: MkdirAll %q: %w", p.Name, err)
	}
	if p.Mode != 0 {
		if err := fsys.Chmod(p.Name, p.Mode.Perm()); err != nil {
			return fmt.Errorf("CreateDir: Chmod %q: %w", p.Name, err)
		}
	}
	if !p.Mtime.IsZero() {
		if err := fsys.Chtimes(p.Name, p.Mtime, p.Mtime); err != nil {
			return fmt.Errorf("CreateDir: Chtimes %q: %w", p.Name, err)
		}
	}
	return nil
}

type CreateFile[F File, Fs Fsys[F]] struct {
	Name    string
	Mode    fs.FileMode
	Mtime   time.Time
	Content []byte
}

func (p *CreateFile[F, Fs]) Path() string {
	return p.Name
}

func (p *CreateFile[F, Fs]) Order() int {
	return 1
}

func (p *CreateFile[F, Fs]) Setup(fsys Fs) error {
	if err := mkdirParent(fsys, p.Name); err != nil {
		return fmt.Errorf("CreateFile: %w", err)
	}

	f, err := fsys.Create(p.Name)
	if err != nil {
		return fmt.Errorf("CreateFile: Create %q: %w", p.Name, err)
	}

	if _, err := f.Write(p.Content); err != nil {
		_ = f.Close()
		_ = fsys.Remove(p.Name)
		return fmt.Errorf("CreateFile: Write %q: %w", p.Name, err)
	}

	if p.Mode != 0 {
		if err := f.Chmod(p.Mode.Perm()); err != nil {
			_ = f.Close()
			_ = fsys.Remove(p.Name)
			return fmt.Errorf("CreateFile: Chmod %q: %w", p.Name, err)
		}
	}

	if err := f.Close(); err != nil {
		_ = fsys.Remove(p.Name)
		return fmt.Errorf("CreateFile: Close %q: %w", p.Name, err)
	}

	if !p.Mtime.IsZero() {
		if err := fsys.Chtimes(p.Name, p.Mtime, p.Mtime); err != nil {
			_ = fsys.Remove(p.Name)
			return fmt.Errorf("CreateFile: Chtimes %q: %w", p.Name, err)
		}
	}

	return nil
}

type CreateSymlink[F File, Fs Fsys[F]] struct {
	Name       string
	TargetPath string
}

func (p *CreateSymlink[F, Fs]) Path() string {
	return p.Name
}

func (p *CreateSymlink[F, Fs]) Order() int {
	return 3
}

func (p *CreateSymlink[F, Fs]) Setup(fsys Fs) error {
	if err := mkdirParent(fsys, p.Name); err != nil {
		return fmt.Errorf("CreateSymlink: %w", err)
	}
	if err := fsys.Symlink(p.TargetPath, p.Name); err != nil {
		return fmt.Errorf("CreateSymlink: Symlink %q -> %q: %w", p.Name, p.TargetPath, err)
	}
	return nil
}

type CreateLink[F File, Fs Fsys[F]] struct {
	Name       string
	TargetPath string
}

func (p *CreateLink[F, Fs]) Path() string {
	return p.Name
}

func (p *CreateLink[F, Fs]) Order() int {
	return 2
}

func (p *CreateLink[F, Fs]) Setup(fsys Fs) error {
	if err := mkdirParent(fsys, p.Name); err != nil {
		return fmt.Errorf("CreateLink: %w", err)
	}
	if err := fsys.Link(p.TargetPath, p.Name); err != nil {
		return fmt.Errorf("CreateLink: Link %q -> %q: %w", p.Name, p.TargetPath, err)
	}
	return nil
}
