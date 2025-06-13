# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- **Testing**: Use `go test -timeout 2s ./...` to run all tests with verbose output
- **Race testing**: Use `go test -race -timeout 2s ./...` to run tests with race detection (slower but finds timing issues)
- **Single test**: Use `go test -timeout 2s -run TestName -v ./PackageName` to run a specific test
- **Single test with race detection**: Use `go test -race -timeout 2s -run TestName -v ./PackageName` to run a specific test with race detection
- **Build**: Use `go build` (requires Go 1.25+)
- **Type checking**: Use `go vet ./...` to check for static analysis issues

## Architecture

This is a Go library (`github.com/ngicks/go-fsys-helper/vroot`) that provides filesystem abstraction with security constraints - particularly preventing path escapes and controlling access boundaries.

### Core Components

**Main Interfaces**:

- `Fs`: Core filesystem interface with OS-like operations (Chmod, Create, Open, etc.)
- `Rooted`: Filesystem that prevents escaping root via path traversal or symlinks
- `Unrooted`: Allows symlink escapes but still blocks path traversal
- `File`: OS-like file interface with Read/Write/Seek operations

**Key Implementations**:

- `osfs/`: Adapters for `*os.Root` to `vroot.Rooted/Unrooted` interfaces
- `iofs_from.go`: Converts `fs.FS` to `vroot.Fs` interfaces (widens capabilities)
- `iofs_to.go`: Converts `vroot.Fs` to standard `fs.FS` interfaces (narrows capabilities)

**Security Features**:

- Path escape detection with `ErrPathEscapes`
- Symlink following with configurable root constraints
- Path traversal prevention

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
