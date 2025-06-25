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

`synthfs` synthesizes file-like view from different data sources into an imitate filesystem.
It can even borrow file-view from `fs.FS` or any implementation.

Example:

- Virtually splitted / concatenated files.
- Virtually compressed / decompressed files.
- Mixing files from different directories/filesystems and/or virtual in-memory contents:
  - place files from 2-3 different `fs.FS` instances into a directory.
  - Take hashsums to expose it as virtual in-memory file in the same directory.

The main purpose of `synthfs` is to let modified content be passed to functions like [tar.Writer.AddFS](https://pkg.go.dev/archive/tar#Writer.AddFS), [zip.Writer.AddFS](https://pkg.go.dev/archive/zip@go1.24.4#Writer.AddFS) without writing contents to a temporary directory.

#### Basic Usage

```go
import "github.com/ngicks/go-fsys-helper/vroot/synthfs"

// Create synthetic filesystem with memory allocator for new files
allocator := synthfs.NewMemFileAllocator(clock.RealWallClock())
mask := fs.FileMode(0o022)
rooted := synthfs.NewRooted("synth://", allocator, synthfs.Option{
    Clock: clock.RealWallClock(),
    Umask: &mask, // Optional umask
})

// New files use the allocator (memory-backed in this case)
file, err := rooted.Create("dynamic.txt")

// Add file views from other sources
fsView, _ := synthfs.NewFsFileView(embedFS, "static/data.txt")
rooted.AddFile("resources/data.txt", fsView, 0o755, 0o644)

// Add entire vroot filesystem
rooted.AddFs("imported", anotherVrootFs, 0o755)
```

#### Use Cases

- **Hybrid Filesystems**: Mix static embedded files with dynamic user files
- **Virtual Filesystems**: Create filesystems that don't map to disk layout
- **Testing with Real Files**: Use actual file views for integration tests
- **Resource Overlays**: Overlay user modifications over read-only resources
- **Custom Storage**: Implement FileView for database, network, or cloud storage

### 3. Memory Filesystem (`memfs/`)

A convenience wrapper around synthfs that provides ready-to-use in-memory filesystems with sensible defaults.

```go
import "github.com/ngicks/go-fsys-helper/vroot/memfs"

// Create in-memory rooted filesystem
memRoot := memfs.NewRooted()

// Create in-memory unrooted filesystem
memUnrooted := memfs.NewUnrooted()

// Use like any filesystem - all operations happen in memory
err := memRoot.Mkdir("tempdir", 0755)
file, err := memRoot.Create("tempdir/data.txt")
```

#### Use Cases

- **Unit Testing**: Test filesystem operations without disk I/O
- **Temporary Workspaces**: Process files without persistence
- **Sandboxing**: Complete isolation from host filesystem
- **Performance**: Eliminate disk I/O bottlenecks

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

The overlay package provides sophisticated union mount capabilities with copy-on-write semantics:

### Basic Overlay Setup

```go
import "github.com/ngicks/go-fsys-helper/vroot/overlayfs"

// Create writable top layer
top, err := osfs.NewRooted("top/data")
if err != nil {
    log.Fatal(err)
}
topMetaFsys, err := osfs.NewRooted("top/meta")
// Setup metadata store for tracking changes
topMeta := overlayfs.NewMetadataStoreSimpleText(topMetaFsys)

// Create read-only lower layers
Layer1Data, err := osfs.NewRooted("layer1/data")
Layer1MetaFsys, err := osfs.NewRooted("layer1/meta")
...

// Create overlay filesystem
overlayFs, err := overlayfs.New(
    // Writable layer
    NewLayer(NewMetadataStoreSimpleText(topMeta), top),
    // Read-only lower layers
    []Layer{layer1, layer2},
    // Option; nil for default
    nil,
)

/*
layouts:

+--------+
|  top   |
+--------+
| layer2 |
+--------+
| layer1 |
+--------+
*/
```

### Overlay Features

- **Union Mount**: Files resolve from top layer down
- **Copy-on-Write**: Lower layer files copied to top layer before any write operation or **opening as writable**
- **Whiteout Support**: Track file deletions across layers
- **Metadata Tracking**: Persistent storage of overlay metadata

```go
// File resolution order: topLayer → lowerLayer1 → lowerLayer2
file, err := overlayFs.Open("config.yaml")

// Modifications trigger copy-up from lower layers
err = overlayFs.Remove("system-file.txt") // Creates whiteout entry
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

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
    // For os-backed fsys
	acceptancetest.MakeOsFsys(tempDir, false, true)
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
