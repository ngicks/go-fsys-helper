# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Mandatory

- If I say "Explain"(case insensitive) I am just needing explanation. DO NOT FIX at this point.

## Commands

- **Change directory**: Before executing any `go` command, `cd` into that `go module`. Don't run them on `./`. Each time after command is done, move back to root dir.
- **Testing**: Use `go test -timeout 2s ./...` to run all tests with verbose output
- **Race testing**: Use `go test -race -timeout 2s ./...` to run tests with race detection (slower but finds timing issues)
- **Single test**: Use `go test -timeout 2s -run TestName -v ./path/to/package` to run a specific test
- **Single test with race detection**: Use `go test -race -timeout 2s -run TestName -v ./path/to/package` to run a specific test with race detection
- **Formatting / Clean imports**: Use `goimports -w .` to clean imports. Everytime you add or remove lines, run this.
- **Type checking**: Use `go vet ./...` to check for static analysis issues
  - also `GOOS=windows go vet ./...` must be called.

## Architecture

This is a Go library (`github.com/ngicks/go-fsys-helper`) that provides filesystem helper utilities across multiple independent modules. Each major package is its own Go module with isolated dependencies.

### Active Packages

**vroot/**: Virtual root filesystem with security constraints (Go 1.25rc1)

- **Primary package** with comprehensive filesystem abstraction
- `Fs`: Core filesystem interface with OS-like operations (Chmod, Create, Open, etc.)
- `Rooted`: Strict containment preventing path traversal and symlink escapes
- `Unrooted`: Allows symlink escapes but blocks path traversal
- Multiple implementations:
  - `osfs/`: OS filesystem wrappers (Rooted and Unrooted)
  - `synthfs/`: Synthetic filesystem combining different data sources
  - `memfs/`: In-memory filesystem for testing
  - `overlayfs/`: Union mount with copy-on-write semantics
- Security models for different containment needs
- Extensive acceptance testing framework

**tarfs/**: TAR archive filesystem implementation (Go 1.24.0)

- Read-only filesystem interface for TAR archives
- Implements `fs.FS` for TAR file contents
- Handles sparse files, symlinks, and hardlinks
- Files implement `io.ReaderAt` and `io.Seeker`
- **Status**: Work-in-progress but functional

**fsutil/**: Filesystem abstraction utilities (Go 1.24.0)

- Interoperable utilities for filesystem abstraction libraries
- Compatible with afero, go-billy, hackpadfs, and vroot
- Interface definitions for filesystem operations
- Utility functions for path operations and error handling

**stream/**: Stream helpers (Go 1.22.0)

- Utilities for `io.Reader`/`io.Writer` operations
- `NewCancellable`: Context-cancellable readers
- `NewMultiReadAtSeekCloser`: Virtual concatenation of `io.ReaderAt`
- `NewByteRepeater`: Infinite repeating byte reader
- No external dependencies (pure stdlib)

### Abandoned Packages

**aferofs/**: Afero filesystem adapters (Go 1.23.0)

- **Status**: FROZEN/ABANDONED - author is moving away from afero
- Previously bridged between afero and other filesystem abstractions
- Excluded from CI pipeline

**Testing Strategy**:

- `acceptancetest/`: Common test scenarios and filesystem fixtures
- Individual `*_test.go` files test specific components
- Tests use temporary directories with predefined file structures
- Use `./working` directory for temporary test files when needed

## Coding Preferences

**Test Code Style**:

- Use `must(osfs.NewRooted(path))` pattern instead of error handling for test setup that should always succeed
- Define named `testCase` struct types instead of anonymous struct literals in test tables
- Panic on fundamental test setup failures rather than using `t.Fatalf`

## Path handling

- Wrap every path with `fileapth.FromSlash` when passing paths to functions.
- Wrap every path with `filepath.ToSlash` when using as test results(including `// Output:` comment in Example tests).

## Opened things handling

- Close every opened thing right after becoming unsed.

## After you think you are done

- Run test for an entire module to check implementation is ok.
- Run tests with race detection (`go test -race -timeout 2s`) to catch timing issues and race conditions
- Run `go vet ./...` to check for static analysis issues and type checking problems
- If test fails and it is by the code you've made, fix the code
- If failure is happening from user-written code, alert user about that and receive further instruction.
