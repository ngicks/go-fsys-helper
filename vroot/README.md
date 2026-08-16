# vroot - Virtual Root Filesystem Abstraction

[![Go Reference](https://pkg.go.dev/badge/github.com/ngicks/go-fsys-helper/vroot.svg)](https://pkg.go.dev/github.com/ngicks/go-fsys-helper/vroot)

The `vroot` package provides a filesystem abstraction layer that builds upon Go's `*os.Root` concept, offering a consistent interface for working with virtual filesystem roots. It delivers a comprehensive filesystem interface similar to `*os.File` with seamless integration for various filesystem implementations and root containment strategies.

## 🎯 Key Features

- **📁 Filesystem Abstraction**: Unified interface based on `*os.Root` for various filesystem implementations
- **🔒 Root Containment Models**: Choose between rooted (strict) or unrooted (relaxed) path containment
- **🔄 Overlay Filesystem**: Union mount with copy-on-write semantics, in its own module ([`vroot-adapter/overlayfs`](../vroot-adapter/overlayfs))
- **🔀 Synthetic Filesystem**: Combine files from different sources into unified filesystem trees
- **💾 In-Memory Storage**: Pure memory-based implementations for testing and isolation
- **📚 Standard Library Integration**: Convert interface to/from `io/fs`
- **🧪 Comprehensive Testing**: Extensive test suite with acceptance tests for all implementations

## 📋 Table of Contents

- [📦 Installation](#-installation)
- [🚀 Quick Start](#-quick-start)
  - [Basic Usage with OS Filesystem](#basic-usage-with-os-filesystem)
  - [Read-Only Filesystem](#read-only-filesystem)
  - [Convert to/from fs.FS](#convert-tofrom-fsfs)
- [🧠 Core Concepts](#-core-concepts)
  - [Interfaces](#interfaces)
  - [Fs Interface](#fs-interface)
  - [Rooted Interface](#rooted-interface)
  - [Unrooted Interface](#unrooted-interface)
- [❓ Why Unrooted?](#-why-unrooted)
- [📂 Implementation Types](#-implementation-types)
  - [1. OS Filesystem (osfs/)](#1-os-filesystem-osfs)
  - [2. Synthetic Filesystem (synthfs/)](#2-synthetic-filesystem-synthfs)
  - [3. Memory Filesystem (memfs/)](#3-memory-filesystem-memfs)
  - [4. Read-Only Wrappers](#4-read-only-wrappers)
  - [5. io/fs Adapters](#5-iofs-adapters)
- [🔄 Overlay Filesystem](#-overlay-filesystem)
- [🧪 Testing](#-testing)
  - [Test Your Implementation](#test-your-implementation)
- [🤝 Contributing](#-contributing)

## 📦 Installation

```bash
go get github.com/ngicks/go-fsys-helper/vroot
```

## 🚀 Quick Start

### Basic Usage with OS Filesystem

```go
package vroot_test

import (
    "errors"
    "fmt"
    "os"

    "github.com/ngicks/go-fsys-helper/vroot"
    "github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func Example_basic_usage() {
    tempDir, err := os.MkdirTemp("", "")
    if err != nil {
        panic(err)
    }

    root, err := osfs.NewRooted(tempDir)
    if err != nil {
        panic(err)
    }
    defer root.Close()

    // File operations within the virtual root
    file, err := root.Create("example.txt")
    if err != nil {
        _ = file.Close()
        panic(err)
    }
    _ = file.Close()

    // Path traversal is contained within the root
    _, err = root.Open("../../../etc/passwd")
    fmt.Printf("path traversal: %v\n", err)
    // path traversal: openat ../../../etc/passwd: path escapes from parent

    // Symlinks can be created
    err = root.Symlink("../../../etc/passwd", "link")
    if err != nil {
        panic(err)
    }

    // But following them respects root boundaries
    _, err = root.Open("link")
    fmt.Printf("symlink resolution: %v\n", err)
	// symlink resolution: openat link: path escapes from parent
}
```

### Read-Only Filesystem

```go
import "github.com/ngicks/go-fsys-helper/vroot"

// Wrap any filesystem to make it read-only
readOnlyFs := vroot.ReadOnlyRooted(rootedFs)

// Write operations will return syscall.EROFS
_, err := readOnlyFs.Create("readonly.txt") // Error: read-only file system
```

### Convert to/from fs.FS

```go
import (
    "embed"
    "os"
    "path/filepath"
    "testing/fstest"

    "github.com/ngicks/go-fsys-helper/vroot"
    "github.com/ngicks/go-fsys-helper/vroot/osfs"
)

//go:embed static/*
var embedFS embed.FS

// Convert fs.FS to vroot (read-only)
vrootReadOnly := vroot.FromIoFsRooted(embedFS)

// Convert vroot to fs.FS
rootedFs, _ := osfs.NewRooted("/some/path")
standardFs := vroot.ToIoFsRooted(rootedFs)

// Test with standard library
fstest.TestFS(standardFs, "file1.txt", "subdir/file2.txt")
```

## 🧠 Core Concepts

### Interfaces

The package defines three main interfaces:

#### `Fs` Interface

Core filesystem operations similar to `*os.Root`:

```go
type Fs interface {
    Chmod(name string, mode fs.FileMode) error
    Create(name string) (File, error)
    Open(name string) (File, error)
    OpenFile(name string, flag int, perm fs.FileMode) (File, error)
    Mkdir(name string, perm fs.FileMode) error
    Remove(name string) error
    Stat(name string) (fs.FileInfo, error)
    // ... and more
}
```

One important change: `ReadLink` instead of `Readlink`. This is changed to align with `fs.ReadLinkFS`.

#### `Rooted` Interface

Rooted prohibits both path traversal and symlink escapes:

```go
type Rooted interface {
    Fs
    OpenRoot(name string) (Rooted, error) // Create sub-root
}
```

#### `Unrooted` Interface

Relaxed interface where symlink escapes are allowed but blocking path traversal:

```go
type Unrooted interface {
    Fs
    OpenUnrooted(name string) (Unrooted, error) // Create sub-filesystem
}
```

## ❓ Why Unrooted?

The `Unrooted` interface exists to address real-world development scenarios where strict containment is too restrictive:

**Development Workflows**: Many development environments rely on symlinks that point outside the project directory. For example:

- Package managers that symlink to shared dependencies
- Build tools that reference external toolchains
- Version control systems with worktrees or submodules
- IDE configurations that link to system libraries

**Practical Example**: Consider a project structure where you're working in `/home/user/project`, but you have:

- A symlink to shared libraries: `./vendor -> /opt/shared/libs`
- Build tools symlinked: `./tools/compiler -> /usr/local/bin/custom-compiler`
- Configuration linked from home: `./config -> ~/.config/myapp`

With `Rooted`, all these symlinks would fail to resolve. `Unrooted` allows these symlinks to work while still preventing direct path traversal attempts like `../../../etc/passwd`.

**The Balance**: `Unrooted` provides a practical middle ground - it prevents accidental (or even malicious) path traversal while allowing intentional symlink usage that many development workflows depend on.

## 📂 Implementation Types

### 1. OS Filesystem (`osfs/`)

#### `osfs.Rooted`

```go
// Wraps *os.Root for maximum security
root, err := osfs.NewRooted("/safe/path")
```

#### `osfs.Unrooted`

```go
// Custom implementation allowing symlink escapes
unrooted, err := osfs.NewUnrooted("/project/path")
```

### 2. Synthetic Filesystem (`synthfs/`)

`synthfs` is an in-memory directory tree whose regular-file bytes are supplied by a `FileView` — a pointer to content in some backing storage (in-memory buffer, an `fs.FS` file, a byte range of another view, generated bytes, …). It lets callers assemble a tree that mixes heterogeneous sources without copying anything to disk.

Use cases:

- Mix files from several `fs.FS` (e.g. multiple `embed.FS`) into one directory.
- Drop a generated file (hash sum, manifest, …) alongside imported content.
- Slice a large file into a virtual sub-range without copying.

The headline use case is feeding such a mixed tree to [`tar.Writer.AddFS`](https://pkg.go.dev/archive/tar#Writer.AddFS) / [`zip.Writer.AddFS`](https://pkg.go.dev/archive/zip#Writer.AddFS) (via `vroot.ToIoFsRoot`) **without** staging to a temp directory.

#### Basic Usage

```go
import (
    "archive/tar"
    "github.com/ngicks/go-fsys-helper/vroot"
    "github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

// nil Option uses defaults: real clock, in-memory allocator, 0o022 umask.
r := synthfs.NewRoot("synth://", nil)

// New files written through the filesystem interface use the allocator.
f, _ := r.Create("dynamic.txt")
_ = f.Close()

// Bind existing content as views — no copy.
r.AddFS("pkg", embedA, nil)               // source A under pkg/
r.AddFS("pkg", embedB, nil)               // source B merges into pkg/
r.AddFile("pkg/SHA256SUMS",               // generated file alongside
    synthfs.NewBytesView(sum, 0o644, now), nil)

// Stream the result without temp files.
tw := tar.NewWriter(w)
_ = tw.AddFS(vroot.ToIoFsRoot(r))
```

#### Resolvers (`AddFile` / `AddFS`)

Both ingest calls take an `AddFunc` resolver. `nil` ⇒ `MergeOverwrite`: directories merge, leaves overwrite. Other built-ins:

- `MergeKeep` — first-source-wins (existing leaf wins on conflict).
- `FailOnConflict` — dirs merge; a leaf collision returns `fs.ErrExist`.

A resolver returns `(AddDecision, error)`. Non-nil error aborts the whole `AddFile`/`AddFS` and propagates. Decision is one of:

- `AddDecisionSkip` — drop the incoming entry; for a directory, its whole subtree is pruned.
- `AddDecisionKeep` — directories merge (reuse + descend); leaves keep the existing entry.
- `AddDecisionOverride` — replace whatever is at the path with the incoming entry.

#### Built-in Views

- `NewMemAllocator(c)` — default writable in-memory allocator (used by `Create`/`OpenFile(O_CREATE)`).
- `NewBytesView(b, mode, mtime)` — read-only view over an existing `[]byte`.
- `NewFsView(fsys, name)` — read-only view backed by a file in any `fs.FS`.
- `NewRangedView(inner, off, n)` — read-only byte-range slice of another view.

### 3. Memory Filesystem (`memfs/`)

A one-liner convenience wrapper around `synthfs` that uses every default.

```go
import "github.com/ngicks/go-fsys-helper/vroot/memfs"

r := memfs.New("memfs://")
_ = r.Mkdir("tempdir", 0o755)
f, _ := r.Create("tempdir/data.txt")
_ = f.Close()
```

`memfs.New(name)` is exactly `synthfs.NewRoot(name, nil)`. Use `synthfs` directly when you need to customize the allocator, umask, clock, or chmod mask.

#### Use Cases

- **Unit Testing**: filesystem-shaped tests without disk I/O.
- **Temporary Workspaces**: process files without persistence.
- **Sandboxing**: complete isolation from the host filesystem.

### 4. Read-Only Wrappers

Transform any filesystem into read-only:

```go
import "github.com/ngicks/go-fsys-helper/vroot"

// Make any filesystem read-only
readOnly := vroot.ReadOnlyRooted(existingRootedFs)
readOnly := vroot.ReadOnlyUnrooted(existingUnrootedFs)

// Write operations return syscall.EROFS or syscall.EPERM
```

### 5. io/fs Adapters

#### From io/fs to vroot

```go
import "github.com/ngicks/go-fsys-helper/vroot"

// Convert fs.ReadLinkFS to vroot.Rooted (read-only)
vrootFs := vroot.FromIoFsRooted(embedFS)
```

#### From vroot to io/fs

```go
// Convert vroot.Rooted to standard fs.FS
standardFs := vroot.ToIoFsRooted(rootedFs)
```

## 🔄 Overlay Filesystem

The overlay filesystem no longer lives in this module. It was rewritten and
relocated to its own module,
[`github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs`](../vroot-adapter/overlayfs),
whose README documents it in full.

`overlayfs` is a union mount over `vroot` backends: a writable **top** layer
stacked over read-only **lower** layers, exposed as a
`vroot.Root[vroot.File, *overlayfs.Fs]`. Reads resolve in mount order (top,
then `lowers[0]`, `lowers[1]`, …); writes land in the top after the lower entry
is copied up; deletions become whiteouts instead of touching the read-only
lowers. Unlike kernel overlayfs and fuse-overlayfs, masking state never appears
as files inside a layer — each layer's whiteouts and opaque directories are
kept in a sqlite database beside its content, reached through a VFS built on
the layer's own `vroot.Fs`.

The move is why the package is a separate module: the sqlite dependency stays
out of `vroot`'s dependency graph, matching the rule the other
`vroot-adapter/*` modules follow.

> **The old `vroot/overlayfs` API was replaced wholesale, not moved.** Nothing
> from it — `Layer`, `NewLayer`, `MetadataStoreLog`, `MetadataStoreMem`,
> `NewCopyUpPolicyStage`, `NewCopyUpPolicyDotTmp`, `(*Fs).Sync` — carries over,
> and `New` takes a different signature. The new module builds layers from
> `DataSource` values instead of `Layer` ones, and has no `Sync` because
> masking is durable by the time the call that changed it returns.

## 🧪 Testing

The package includes comprehensive acceptance tests in the `acceptancetest/` directory:

```bash
# Run all tests
go test ./...

# Run specific implementation tests
go test ./osfs/
go test ./synthfs/
go test ./memfs/
```

### Test Your Imlementation

```go
package osfs

import (
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
    // For os-backed fsys
	setupFs, err := NewFs(tempDir)
	if err != nil {
		panic(err)
	}
	acceptancetest.MakeOsFsys(testhelper.New(t, setupFs), false, true)
    // For readonly testing
    {
	    r, err := NewRooted(filepath.Join(tempDir, "root", "readable"))
	    if err != nil {
		    panic(err)
	    }
	    defer r.Close()
	    acceptancetest.RootedReadOnly(t, r)
    }
    // For read-write testing
    {
	    r, err := NewRooted(filepath.Join(tempDir, "root", "writable"))
	    if err != nil {
		    panic(err)
	    }
	    defer r.Close()
	    acceptancetest.RootedReadWrite(t, r)
    }
    // For more stricter reader-side test, use fstest.TestFS
    {
        r, err := NewRooted(filepath.Join(tempDir, "root", "readable"))
	    if err != nil {
		    panic(err)
	    }
	    defer r.Close()
	    fsys := vroot.ToIoFsRooted(r)
	    fstest.TestFS(fsys, acceptancetest.RootFsysReadableFiles...)
    }

    // Use Unrooted* equivalent to test Unrooted imlementations.
}
```

## 🤝 Contributing

Contributions are welcome! Please ensure:

1. Sign all commits.
1. All tests pass: `go test ./...` and with -race flag `go test -race ./...`

Currently nothing has timing related tests(except for `fstest.TestFS`) but maybe I'll add ones later

(Maybe I'll expand this section later...)
