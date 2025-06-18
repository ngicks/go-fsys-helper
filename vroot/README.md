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

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Security Models](#security-models)
- [Implementation Types](#implementation-types)
- [Overlay Filesystem](#overlay-filesystem)
- [API Reference](#api-reference)
- [Examples](#examples)
- [Security Considerations](#security-considerations)
- [Testing](#testing)

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

// Can also add file views from other sources (when AddFile is implemented)
// fsView, _ := synthfs.NewFsFileView(embedFS, "static/data.txt")
// rooted.AddFile("resources/data.txt", fsView)
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
import "github.com/ngicks/go-fsys-helper/vroot/overlay"

// Create writable top layer
top, err := osfs.NewRooted("top/data")
if err != nil {
    log.Fatal(err)
}
topMetaFsys, err := osfs.NewRooted("top/meta")
// Setup metadata store for tracking changes
topMeta := overlay.NewMetadataStoreSimpleText(topMetaFsys)

// Create read-only lower layers
Layer1Data, err := osfs.NewRooted("layer1/data")
Layer1MetaFsys, err := osfs.NewRooted("layer1/meta")
...

// Create overlay filesystem
overlayFs, err := overlay.New(
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
- **Copy-on-Write**: Lower layer files copied to top layer before modification
- **Whiteout Support**: Track file deletions across layers
- **Metadata Tracking**: Persistent storage of overlay metadata

```go
// File resolution order: topLayer → lowerLayer1 → lowerLayer2
file, err := overlayFs.Open("config.yaml")

// Modifications trigger copy-up from lower layers
err = overlayFs.Remove("system-file.txt") // Creates whiteout entry
```

## 📖 API Reference

### Core Operations

```go
// File operations
file, err := fs.Create("newfile.txt")
file, err := fs.Open("existing.txt")
file, err := fs.OpenFile("file.txt", os.O_RDWR, 0644)

// Directory operations
err := fs.Mkdir("newdir", 0755)
err := fs.Remove("file-or-dir")
entries, err := fs.ReadDir("directory")

// Metadata operations
info, err := fs.Stat("file.txt")      // Follow symlinks
info, err := fs.Lstat("symlink.txt")  // Don't follow symlinks
err := fs.Chmod("file.txt", 0644)
err := fs.Chtimes("file.txt", atime, mtime)

// Symlink operations
err := fs.Symlink("target", "linkname")
target, err := fs.Readlink("symlink")
```

### File Interface

```go
// Reading and writing
n, err := file.Read(buffer)
n, err := file.Write(data)
n, err := file.ReadAt(buffer, offset)
n, err := file.WriteAt(data, offset)

// Navigation and metadata
offset, err := file.Seek(0, io.SeekStart)
info, err := file.Stat()
entries, err := file.ReadDir(-1) // For directories

// Resource management
err := file.Close()
```

## 💡 Examples

### Web Server with Virtual Root

```go
func setupWebServer() http.Handler {
    // Create virtual root for document serving
    docRoot, err := osfs.NewRooted("/var/www/html")
    if err != nil {
        log.Fatal(err)
    }

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Serve files from virtual root - path traversal naturally contained
        file, err := docRoot.Open(r.URL.Path)
        if err != nil {
            http.NotFound(w, r)
            return
        }
        defer file.Close()

        // Serve file...
    })
}
```

### Build Environment with Different Roots

```go
func createBuildEnvironment(srcPath, outputPath string) error {
    // Source code with relaxed containment (follows external symlinks)
    src, err := osfs.NewUnrooted(srcPath)
    if err != nil {
        return err
    }

    // Output directory with strict containment
    output, err := osfs.NewRooted(outputPath)
    if err != nil {
        return err
    }

    // Build process operates within defined roots
    return runBuild(src, output)
}
```

### Development Overlay

```go
func setupDevEnvironment() (vroot.Rooted, error) {
    // System base layer (read-only)
    systemLayer, err := osfs.NewRooted("/usr/share/myapp")
    if err != nil {
        return nil, err
    }

    // User modifications layer (writable)
    userLayer, err := osfs.NewRooted("/home/user/.myapp")
    if err != nil {
        return nil, err
    }

    // Create overlay for development
    return overlay.New(
        userLayer,
        overlay.NewLayers(vroot.ReadOnlyRooted(systemLayer)),
        overlay.NewMetadataStoreSimpleText(),
        overlay.CopyPolicy{},
    )
}
```

### In-Memory Testing

```go
import "github.com/ngicks/go-fsys-helper/vroot/memfs"

func TestFileProcessor(t *testing.T) {
    // Create isolated in-memory filesystem for test
    fs := memfs.NewRooted()

    // Setup test data
    err := fs.Mkdir("input", 0755)
    if err != nil {
        t.Fatal(err)
    }

    // Create test file
    file, err := fs.Create("input/data.txt")
    if err != nil {
        t.Fatal(err)
    }
    file.Write([]byte("test data"))
    file.Close()

    // Run processor with in-memory filesystem
    err = processFiles(fs)
    if err != nil {
        t.Fatal(err)
    }

    // Verify output
    output, err := fs.ReadFile("output/result.txt")
    if err != nil {
        t.Fatal(err)
    }

    // Assert results without any disk I/O
    if string(output) != expectedResult {
        t.Errorf("got %q, want %q", output, expectedResult)
    }
}
```

### Synthetic Filesystem with Mixed Sources

```go
import (
    "embed"
    "github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

//go:embed static/*
var staticFiles embed.FS

func createHybridFilesystem() vroot.Rooted {
    // Create filesystem that uses memory for new files
    allocator := synthfs.NewMemFileAllocator(clock.RealWallClock())
    fs := synthfs.NewRooted("hybrid://", allocator, synthfs.Option{})

    // Future: Add static files from embedded filesystem
    // When AddFile is implemented, you could do:
    // staticView, _ := synthfs.NewFsFileView(staticFiles, "static/template.html")
    // fs.AddFile("templates/default.html", staticView)

    // Future: Add views from other sources
    // dbView := NewDatabaseFileView(db, "config_table")
    // fs.AddFile("config/settings.json", dbView)

    // New files created with Create() will use memory allocator
    // Static files remain read-only from their original source
    // Database-backed files could be read-write through custom FileView

    return fs
}
```

## 📝 Implementation Notes

### Path Handling

- All implementations validate paths using `filepath.IsLocal()`
- `../` sequences are handled according to the containment model
- Absolute paths are resolved relative to the virtual root

### Symlink Behavior

- **Rooted**: Symlinks are resolved and must stay within the root
- **Unrooted**: Symlinks can point outside the root but path traversal is still prevented
- Loop detection prevents infinite symlink cycles (max 40 iterations)

### Platform Differences

Some implementation details vary by platform:

- `osfs.Rooted` uses `*os.Root` where available (Unix-like systems)
- `osfs.Unrooted` uses custom path validation on all platforms
- `FromIoFsRooted` provides compatibility with `io/fs` interfaces
- js/wasm platform has limited `*os.Root` support

### Error Handling

- `ErrPathEscapes`: Returned when paths escape the virtual root
- Standard `fs.ErrNotExist`, `fs.ErrPermission` for compatibility
- Error messages follow Go conventions for path operations

## 🧪 Testing

The package includes comprehensive acceptance tests in the `acceptancetest/` directory:

```bash
# Run all tests
go test ./...

# Run specific implementation tests
go test ./osfs/
go test ./overlay/
go test ./synthfs/
go test ./memfs/
```

### Test Structure

```
outside/                  # Files outside root (should be inaccessible)
root/
├── readable/            # Read-only test files
│   ├── file1.txt
│   └── subdir/
└── writable/           # Read-write test files
    ├── file2.txt
    └── newdir/
```

Tests cover:

- Path escape prevention
- Symlink resolution and security
- File and directory operations
- Error conditions and edge cases
- Cross-platform compatibility

## 🤝 Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test ./...`
2. Security model is maintained
3. Cross-platform compatibility
4. Comprehensive test coverage for new features

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](../LICENSE) file for details.

---

**Note**: This package provides a consistent abstraction over filesystem roots, building upon Go's `*os.Root` concept. Choose the containment model (`Rooted` vs `Unrooted`) that best fits your use case.
