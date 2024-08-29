package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"iter"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"text/template"

	"github.com/ngicks/go-fsys-helper/aferofs/internal/applygoimports"
	"golang.org/x/tools/go/packages"
)

var (
	dir             = flag.String("dir", "./", "cwd set for go tool chain")
	outSuffix       = flag.String("suffix", ".generate", "source code suffix")
	targetPkg       = flag.String("pkg", "", "target package.")
	targetFsysTypes = flag.String("fsys", "", "target types. comma-separated.")
	targetFileTypes = flag.String("file", "", "target file types. comma-separated.")
)

type TemplateParam struct {
	Target    Target
	Modifiers []modifier
	Methods   Methods
}

type Target struct {
	PackageName string
	TypeName    string
	InnerName   string
	ImplFlags
}

type ImplFlags struct {
	BeforeEach     bool
	AfterEach      bool
	ModifyPath     bool
	ModifyMode     bool
	ModifyTimes    bool
	ModifyErr      bool
	ModifyFile     bool
	ModifyFi       bool
	ModifyDirnames bool
	ModifyP        bool
	ModifyN        bool
	ModifyOff      bool
	ModifyString   bool
}

type Methods []Method

type Method struct {
	Name      string
	Arg       []Field
	Ret       []Field
	Target    Target
	Modifiers []modifier
}

type Field struct {
	Name string
	Type string
}

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedExportFile |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedTypesSizes |
			packages.NeedModule |
			packages.NeedEmbedFiles |
			packages.NeedEmbedPatterns,
		Context: ctx,
		Dir:     *dir,
	}
	pkgs, err := packages.Load(cfg, *targetPkg)
	if err != nil {
		panic(err)
	}

	pkg := pkgs[0]

	err = generate(
		ctx,
		pkg.Types,
		pkg.Fset,
		fileWrapper,
		slices.All(strings.Split(*targetFsysTypes, ",")),
		isAferoFs,
		fsysMethods,
	)
	if err != nil {
		panic(err)
	}

	err = generate(
		ctx,
		pkg.Types,
		pkg.Fset,
		fileWrapper,
		slices.All(strings.Split(*targetFileTypes, ",")),
		isAferoFile,
		fileMethods,
	)
	if err != nil {
		panic(err)
	}
}

func generate(
	ctx context.Context,
	pkgTypes *types.Package,
	fset *token.FileSet,
	tmpl *template.Template,
	targetTypes iter.Seq2[int, string],
	checker func(v *types.Var) bool,
	methods Methods,
) error {
	for _, ty := range targetTypes {
		obj := pkgTypes.Scope().Lookup(ty)
		if obj == nil {
			return fmt.Errorf("type not found")
		}
		structTy, ok := obj.Type().Underlying().(*types.Struct)
		if !ok {
			return fmt.Errorf("not a struct type: %v", obj)
		}
		var name string
		for i := 0; i < structTy.NumFields(); i++ {
			field := structTy.Field(i)
			if checker(field) {
				name = field.Name()
				break
			}
		}
		if name == "" {
			return fmt.Errorf("does not have afero.Fs as field: %v", obj)
		}

		p := types.NewPointer(obj.Type())
		ms := types.NewMethodSet(p)

		target := Target{
			PackageName: obj.Pkg().Name(),
			TypeName:    ty,
			InnerName:   name,
			ImplFlags:   implFlags(ms),
		}

		params := TemplateParam{
			Target:    target,
			Methods:   methods,
			Modifiers: modifiers,
		}

		pos := fset.PositionFor(obj.Pos(), true)

		err := write(ctx, ty, pos.Filename, tmpl, params)
		if err != nil {
			return err
		}
	}
	return nil
}

func write(
	ctx context.Context, typeName string,
	filename_ string, tmpl *template.Template, params any,
) error {
	filename, _ := strings.CutSuffix(filepath.Base(filename_), filepath.Ext(filename_))
	filename = filename + "." + strings.ToLower(typeName) + *outSuffix + filepath.Ext(filename_)

	fileDir := filepath.Dir(filename_)
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absDir, fileDir)
	if err != nil {
		return err
	}

	outFile, err := os.Create(filepath.Join(rel, filename))
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	go func() {
		err := tmpl.Execute(pw, params)
		pw.CloseWithError(err)
	}()
	r, err := applygoimports.ApplyGoimportsPiped(ctx, pr)
	// r := pr
	if err != nil {
		_ = outFile.Close()
		_ = pr.CloseWithError(err)
		return err
	}

	_, err = io.Copy(outFile, r)
	_ = pr.CloseWithError(err)
	if err != nil {
		_ = outFile.Close()
		return err
	}
	err = r.Close()
	if err != nil {
		_ = outFile.Close()
		return err
	}
	err = outFile.Sync()
	_ = outFile.Close()
	if err != nil {
		return err
	}
	return nil
}

func isAferoFs(v *types.Var) bool {
	named, ok := v.Type().(*types.Named)
	if !ok {
		return false
	}
	return named.Obj().Pkg().Path() == "github.com/spf13/afero" && named.Obj().Name() == "Fs"
}

func isAferoFile(v *types.Var) bool {
	named, ok := v.Type().(*types.Named)
	if !ok {
		return false
	}
	return named.Obj().Pkg().Path() == "github.com/spf13/afero" && named.Obj().Name() == "File"
}
