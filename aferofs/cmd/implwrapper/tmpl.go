package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/types"
	"slices"
	"strconv"
	"text/template"
)

var (
	//go:embed methods.tmpl
	methodsTempl string
)

var (
	fileWrapper = template.Must(template.New("").Funcs(funcMap).Parse(methodsTempl))
)

var funcMap template.FuncMap = template.FuncMap{
	"fieldList": func(f []Field) string {
		var w bytes.Buffer
		for _, f := range f {
			_, _ = fmt.Fprintf(&w, "%s %s, ", f.Name, f.Type)
		}
		if len(f) > 0 {
			w.Truncate(w.Len() - 2)
		}
		return w.String()
	},
	"fieldName": fieldName,
	"fillMethod": func(m Method, t Target, mod []modifier) Method {
		m.Target = t
		m.Modifiers = mod
		return m
	},
	"quote": func(s string) string {
		return strconv.Quote(s)
	},
	"includesErr": func(f []Field) bool {
		return len(f) > 0 && f[len(f)-1].Type == "error"
	},
	"hasName": func(f []Field, name ...string) bool {
		for _, f := range f {
			if slices.Contains(name, f.Name) {
				return true
			}
		}
		return false
	},
	"hasType": func(f []Field, ty ...string) bool {
		for _, f := range f {
			if slices.Contains(ty, f.Type) {
				return true
			}
		}
		return false
	},
	"getName": func(f []Field, name ...string) string {
		for _, f := range f {
			if slices.Contains(name, f.Name) {
				return f.Name
			}
		}
		return ""
	},
	"hasOldname": func(f []Field) bool {
		for _, f := range f {
			if f.Name == "oldname" {
				return true
			}
		}
		return false
	},
}

func fieldName(f []Field) string {
	var w bytes.Buffer
	for _, f := range f {
		_, _ = fmt.Fprintf(&w, "%s, ", f.Name)
	}
	if len(f) > 0 {
		w.Truncate(w.Len() - 2)
	}
	return w.String()
}

func fieldRet(f []Field) string {
	var w bytes.Buffer
	for _, f := range f {
		if len(f.Type) > 0 {
			_, _ = fmt.Fprintf(&w, "%s, ", f.Name)
		} else {
			w.WriteString("_, ")
		}
	}
	if len(f) > 0 {
		w.Truncate(w.Len() - 2)
	}
	return w.String()

}

type modifier struct {
	f      []Field
	Method string
}

func (c modifier) Match(f []Field) bool {
	for _, target := range c.f {
		if slices.Contains(f, target) {
			return true
		}
	}
	return false
}

func (c modifier) Param(f []Field) string {
	if len(f) > 0 && c.f[0].Type == "fs.FileInfo" {
		return "[]fs.FileInfo{" + c.f[0].Name + "}"
	}
	return fieldName(c.f)
}

func (c modifier) Ret(f []Field) string {
	return fieldRet(c.f)
}

func (c modifier) Unwrap(f []Field) string {
	if len(f) > 0 && c.f[0].Type == "fs.FileInfo" {
		return "[0]"
	}
	return ""
}

func (c modifier) Impls(flag ImplFlags) bool {
	return targetImpls(flag, c.Method)
}

var (
	modifiers = []modifier{
		{
			[]Field{{"name", "string"}, {`""`, ""}},
			"modifyPath",
		},
		{
			[]Field{{"path", "string"}, {`""`, ""}},
			"modifyPath",
		},
		{
			[]Field{{"oldname", "string"}, {"newname", "string"}},
			"modifyPath",
		},
		{
			[]Field{{"perm", "fs.FileMode"}},
			"modifyMode",
		},
		{
			[]Field{{"mode", "fs.FileMode"}},
			"modifyMode",
		},
		{
			[]Field{{"atime", "fs.FileMode"}, {"mtime", "time.Time"}},
			"modifyTimes",
		},
		{
			[]Field{{"err", "error"}},
			"modifyErr",
		},
		{
			[]Field{{"f", "afero.File"}},
			"modifyFile",
		},
		{
			[]Field{{"fi", "[]fs.FileInfo"}},
			"modifyFi",
		},
		{
			[]Field{{"fi", "fs.FileInfo"}},
			"modifyFi",
		},
		{
			[]Field{{"s", "[]string"}},
			"modifyDirnames",
		},
		{
			[]Field{{"p", "[]byte"}},
			"modifyP",
		},
		{
			[]Field{{"n", "int"}},
			"modifyN",
		},
		{
			[]Field{{"off", "int64"}},
			"modifyOff",
		},
		{
			[]Field{{"s", "string"}},
			"modifyString",
		},
	}
)

func implFlags(ms *types.MethodSet) ImplFlags {
	var flags ImplFlags
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		switch sel.Obj().Name() {
		case "beforeEach":
			flags.BeforeEach = true
		case "afterEach":
			flags.AfterEach = true
		case "modifyPath":
			flags.ModifyPath = true
		case "modifyMode":
			flags.ModifyMode = true
		case "modifyTimes":
			flags.ModifyTimes = true
		case "modifyFile":
			flags.ModifyFile = true
		case "modifyErr":
			flags.ModifyErr = true
		case "modifyFi":
			flags.ModifyFi = true
		case "modifyDirnames":
			flags.ModifyDirnames = true
		case "modifyP":
			flags.ModifyP = true
		case "modifyN":
			flags.ModifyN = true
		case "modifyOff":
			flags.ModifyOff = true
		case "modifyString":
			flags.ModifyString = true
		}
	}
	return flags
}

func targetImpls(flags ImplFlags, ms string) bool {
	switch ms {
	case "beforeEach":
		return flags.BeforeEach
	case "afterEach":
		return flags.AfterEach
	case "modifyPath":
		return flags.ModifyPath
	case "modifyMode":
		return flags.ModifyMode
	case "modifyTimes":
		return flags.ModifyTimes
	case "modifyFile":
		return flags.ModifyFile
	case "modifyErr":
		return flags.ModifyErr
	case "modifyFi":
		return flags.ModifyFi
	case "modifyDirnames":
		return flags.ModifyDirnames
	case "modifyP":
		return flags.ModifyP
	case "modifyN":
		return flags.ModifyN
	case "modifyOff":
		return flags.ModifyOff
	case "modifyString":
		return flags.ModifyString
	}
	return false
}
