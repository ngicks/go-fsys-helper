# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Rules

- If I say "Explain"(case insensitive) I am just needing explanation. DO NOT FIX at this point.

## Commands

- **Change directory**: Before executing any `go` command, `cd` into that `go module`. Don't run them on `./`. Each time after command is done, move back to root dir.
- **Testing**: Use `go test -timeout 2s ./...` to run all tests with verbose output
- **Race testing**: Use `go test -race -timeout 2s ./...` to run tests with race detection (slower but finds timing issues)
- **Single test**: Use `go test -timeout 2s -run TestName -v ./path/to/package` to run a specific test
- **Single test with race detection**: Use `go test -race -timeout 2s -run TestName -v ./path/to/package` to run a specific test with race detection
- **Formatting / Clean imports**: Use `goimports -w .` to clean imports. Everytime you add or remove lines, run this.
- **Type checking**: Use `go vet ./...` to check for static analysis issues

## Architecture

This is a Go library (`github.com/ngicks/go-fsys-helper`) that provides filesystem helper utilities across multiple packages.

### Packages

**vroot/**: Virtual root filesystem with security constraints

- `Fs`: Core filesystem interface with OS-like operations (Chmod, Create, Open, etc.)
- `Rooted`: Filesystem that prevents escaping root via path traversal or symlinks
- `Unrooted`: Allows symlink escapes but still blocks path traversal
- Path escape detection and symlink following with configurable root constraints

**tarfs/**: TAR archive filesystem implementation

- Provides filesystem interface for TAR archives
- Read-only access to TAR file contents

**aferofs/**: Afero filesystem adapters

- Forget about this. This is abondoned.

**stream/**: Streaming utilities for filesystem operations

**fsutil/**: Filesystem-abstraction-library interaoperable utilities.

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

## After you think you are done

- Run test for an entire module to check implementation is ok.
- Run tests with race detection (`go test -race -timeout 2s`) to catch timing issues and race conditions
- Run `go vet ./...` to check for static analysis issues and type checking problems
- If test fails and it is by the code you've made, fix the code
- If failure is happening from user-written code, alert user about that and receive further instruction.
