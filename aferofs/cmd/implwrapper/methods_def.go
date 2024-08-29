package main

var (
	fsysMethods Methods = []Method{
		{
			Name: "Create",
			Arg:  []Field{{"name", "string"}},
			Ret:  []Field{{"f", "afero.File"}, {"err", "error"}},
		},
		{
			Name: "Mkdir",
			Arg:  []Field{{"name", "string"}, {"perm", "fs.FileMode"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "MkdirAll",
			Arg:  []Field{{"path", "string"}, {"perm", "fs.FileMode"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Open",
			Arg:  []Field{{"name", "string"}},
			Ret:  []Field{{"f", "afero.File"}, {"err", "error"}},
		},
		{
			Name: "OpenFile",
			Arg:  []Field{{"name", "string"}, {"flag", "int"}, {"perm", "fs.FileMode"}},
			Ret:  []Field{{"f", "afero.File"}, {"err", "error"}},
		},
		{
			Name: "Remove",
			Arg:  []Field{{"name", "string"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "RemoveAll",
			Arg:  []Field{{"path", "string"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Rename",
			Arg:  []Field{{"oldname", "string"}, {"newname", "string"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Stat",
			Arg:  []Field{{"name", "string"}},
			Ret:  []Field{{"fi", "fs.FileInfo"}, {"err", "error"}},
		},
		{
			Name: "Name",
			Arg:  []Field{},
			Ret:  []Field{{"name", "string"}},
		},
		{
			Name: "Chmod",
			Arg:  []Field{{"name", "string"}, {"mode", "fs.FileMode"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Chown",
			Arg:  []Field{{"name", "string"}, {"uid", "int"}, {"gid", "int"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Chtimes",
			Arg:  []Field{{"name", "string"}, {"atime", "time.Time"}, {"mtime", "time.Time"}},
			Ret:  []Field{{"err", "error"}},
		},
	}
	fileMethods Methods = []Method{
		{
			Name: "Close",
			Arg:  []Field{},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Name",
			Arg:  []Field{},
			Ret:  []Field{{"s", "string"}},
		},
		{
			Name: "Read",
			Arg:  []Field{{"p", "[]byte"}},
			Ret:  []Field{{"n", "int"}, {"err", "error"}},
		},
		{
			Name: "ReadAt",
			Arg:  []Field{{"p", "[]byte"}, {"off", "int64"}},
			Ret:  []Field{{"n", "int"}, {"err", "error"}},
		},
		{
			Name: "Readdir",
			Arg:  []Field{{"count", "int"}},
			Ret:  []Field{{"fi", "[]fs.FileInfo"}, {"err", "error"}},
		},
		{
			Name: "Readdirnames",
			Arg:  []Field{{"n", "int"}},
			Ret:  []Field{{"s", "[]string"}, {"err", "error"}},
		},
		{
			Name: "Seek",
			Arg:  []Field{{"offset", "int64"}, {"whence", "int"}},
			Ret:  []Field{{"n", "int64"}, {"err", "error"}},
		},
		{
			Name: "Stat",
			Arg:  []Field{},
			Ret:  []Field{{"fi", "fs.FileInfo"}, {"err", "error"}},
		},
		{
			Name: "Sync",
			Arg:  []Field{},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Truncate",
			Arg:  []Field{{"size", "int64"}},
			Ret:  []Field{{"err", "error"}},
		},
		{
			Name: "Write",
			Arg:  []Field{{"p", "[]byte"}},
			Ret:  []Field{{"n", "int"}, {"err", "error"}},
		},
		{
			Name: "WriteAt",
			Arg:  []Field{{"p", "[]byte"}, {"off", "int64"}},
			Ret:  []Field{{"n", "int"}, {"err", "error"}},
		},
		{
			Name: "WriteString",
			Arg:  []Field{{"s", "string"}},
			Ret:  []Field{{"n", "int"}, {"err", "error"}},
		},
	}
)
