# vroot - Virtual Root Filesystem Abstraction

[![Go Reference](https://pkg.go.dev/badge/github.com/ngicks/go-fsys-helper/vroot.svg)](https://pkg.go.dev/github.com/ngicks/go-fsys-helper/vroot)

The `vroot` package provides a filesystem abstraction layer that builds upon Go's `*os.Root` concept, offering a consistent interface for working with virtual filesystem roots. It delivers a comprehensive filesystem interface similar to `*os.File` with seamless integration for various filesystem implementations and root containment strategies.

## 🎯 Key Features

- **📁 Filesystem Abstraction**: Unified interface based on `*os.Root` for various filesystem implementations
- **🔒 Root Containment Models**: Choose between rooted (strict) or unrooted (relaxed) path containment
- **🔄 Overlay Filesystem**: Full union mount implementation with copy-on-write semantics
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
  - [Basic Overlay Setup](#basic-overlay-setup)
  - [Overlay Features](#overlay-features)
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

`overlayfs` is a **union mount**: a writable **top** layer stacked over zero or
more read-only **lower** layers. Reads resolve top→lowers (top wins); writes
land in top after a **copy-up** of the lower entry; deletions become
**whiteouts** rather than mutating the read-only lowers. `*overlayfs.Fs`
implements `vroot.Root[vroot.File, *overlayfs.Fs]`, so it has the full `Fs`
suite plus `OpenRoot` and rooted confinement.

### Basic Overlay Setup

```go
import (
    "github.com/ngicks/go-fsys-helper/vroot"
    "github.com/ngicks/go-fsys-helper/vroot/osfs"
    "github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// Writable top (any vroot.Fs — osfs for on-disk, memfs.New(name) for in-memory).
top, err := osfs.NewRoot("top/data")
if err != nil {
    log.Fatal(err)
}

// Read-only lower layers, ordered low→high priority.
lower1, err := osfs.NewRoot("layer1")
lower2, err := osfs.NewRoot("layer2")

// The top layer carries the overlay's metadata journal (whiteouts / opaque
// dirs). MetadataStoreLog is the default durable store (append-log +
// compaction); use NewMetadataStoreMem() for an ephemeral overlay. The store's
// backing fsys is vroot.Fs[vroot.File], so widen a *osfs.Root with vroot.Widen.
meta := overlayfs.NewMetadataStoreLog(vroot.Widen(top), nil) // nil → defaults (batched fsync)

overlayFs := overlayfs.New(
    // Writable top + its metadata journal. NewLayer erases the file type via
    // vroot.Widen, so heterogeneous layers (osfs *os.File, memfs vroot.File, …)
    // mix freely.
    overlayfs.NewLayer[*os.File](top, meta),
    // Read-only lowers (low→high priority); meta may be nil for lowers.
    []overlayfs.Layer{
        overlayfs.NewLayer[*os.File](lower1, nil),
        overlayfs.NewLayer[*os.File](lower2, nil),
    },
    nil, // *Option; nil → DefaultOption (visible Stage work dir, batched fsync)
)
defer overlayFs.Close() // closes top + lowers, flushes the journal

/*
resolution order:  top → lower2 → lower1   (last lower = highest priority)

+--------+
|  top   |  writable; copy-up target; whiteouts/opaque live in its journal
+--------+
| lower2 |  read-only
+--------+
| lower1 |  read-only
+--------+
*/
```

### Overlay Features

- **Union mount**: files resolve top→lowers (top wins).
- **Copy-on-write**: a lower entry is copied up to top before any write — write-
  open, `Chmod`/`Chown`/`Chtimes`, `Rename`/`Link`. Concurrent writers to the
  same lower-only path collapse to a single copy-up (`singleflight`).
- **Whiteout vs opaque**: `Remove` of a lower-backed entry records a *whiteout*
  (the name, and its lower subtree, are hidden). `rm -rf dir && mkdir dir` over
  lower content makes the recreated dir *opaque* (stale lower children stay
  hidden while new top children show) — fixing the classic delete-then-recreate
  resurrection bug.
- **Copy-up policy**: `NewCopyUpPolicyStage(workDir)` (default) stages temps in
  an explicit, *visible* work dir at the top root then publishes via an atomic
  same-filesystem `Rename`; `NewCopyUpPolicyDotTmp(nameMax)` stages beside the
  destination with no standing dir.
- **Pluggable journal**: `MetadataStoreLog` (append-log + compaction, default
  batched `fsync` off the hot path; force durability with `(*Fs).Sync()`) or
  `MetadataStoreMem` (ephemeral, zero-cost).
- **Fine-grained concurrency**: a sparse table of per-path *overlay virtual
  nodes* (`ovl_node`) owns each touched path's deviation-state and lock, so
  independent paths never serialize. The per-node lock is also the cross-layer
  transaction boundary that keeps a top-write + journal-flip + lower-lookup
  consistent (the backends share no lock). Optional Windows-style
  `ERROR_SHARING_VIOLATION` via `Option.DisableOpenFileRemoval`.

```go
// File resolution order: top → lower2 → lower1
file, err := overlayFs.Open("config.yaml")

// A write-open of a lower-only file copies it up to top first.
w, err := overlayFs.OpenFile("config.yaml", os.O_RDWR, 0)

// Remove of a lower-backed file records a whiteout (the lower copy is untouched).
err = overlayFs.Remove("system-file.txt")

// Force the metadata journal durable at a checkpoint (relevant in batched mode).
err = overlayFs.Sync()
```

## 🧪 Testing

The package includes comprehensive acceptance tests in the `acceptancetest/` directory:

```bash
# Run all tests
go test ./...

# Run specific implementation tests
go test ./osfs/
go test ./overlayfs/
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
